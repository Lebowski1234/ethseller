//This is a service for the Dero Stargate R2 testnet, written for the dARCH 2021 Event 0.5 competition. For full documentation, check out https://github.com/Lebowski1234/ethseller

package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	
	"go.etcd.io/bbolt"

	"github.com/deroproject/derohe/rpc"
	//"github.com/deroproject/derohe/walletapi"
	"github.com/ybbus/jsonrpc"
)

const PLUGIN_NAME = "eth_seller"

const DEST_PORT = uint64(0x2054789213654785)

var expected_arguments = rpc.Arguments{
	{rpc.RPC_DESTINATION_PORT, rpc.DataUint64, DEST_PORT},
	{rpc.RPC_COMMENT, rpc.DataString, "Purchase 0.1 ETH for 1 DERO"}, 
	{rpc.RPC_VALUE_TRANSFER, rpc.DataUint64, uint64(100000)}, // in atomic units

}

// currently the interpreter seems to have a glitch if this gets initialized within the code
// see limitations github.com/traefik/yaegi
var response = rpc.Arguments{
	{rpc.RPC_DESTINATION_PORT, rpc.DataUint64, uint64(0)},
	{rpc.RPC_SOURCE_PORT, rpc.DataUint64, DEST_PORT},
	{rpc.RPC_COMMENT, rpc.DataString, ""},
}

var rpcClient = jsonrpc.NewClient("http://127.0.0.1:40403/json_rpc")
var finishedErrors map[string]bool //so we don't keep displaying same error messages every loop iteration

//convenient struct for DB functions
type DataBase struct {
	db	*bbolt.DB
}


func main() {
	//parse command line flags
	flagNewKeys := flag.Bool("newkeys", false, "Generate new keys")
	flagDisplayKeys := flag.Bool("displaykeys", false, "Display list of keys in DB and sold status")
	flagStart := flag.Bool("start", false, "Start service")
	flagKeyQty := flag.Int("keyqty", 10, "Number of keys to generate, e.g. '-keyqty=10'")
		
	flag.Parse()

	//handle SIGINT and SIGTERM for graceful DB close
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Println(sig)
		done <- true
	}()
	
	//starting service
	var err error
	log.Println("Eth Seller - Service Started")
	var addr *rpc.Address
	var addr_result rpc.GetAddress_Result
	err = rpcClient.CallFor(&addr_result, "GetAddress")
	if err != nil || addr_result.Address == "" {
		log.Printf("Could not obtain address from wallet err %s\n", err)
		return
	}

	if addr, err = rpc.NewAddress(addr_result.Address); err != nil {
		log.Printf("Address could not be parsed: addr:%s err:%s\n", addr_result.Address, err)
		return
	}

	shasum := fmt.Sprintf("%x", sha1.Sum([]byte(addr.String())))

	db_name := fmt.Sprintf("%s_%s.bbolt.db", PLUGIN_NAME, shasum)
	
	db, err := openDB(db_name)	
	if err != nil {
		log.Fatal(err)
	}
	
	log.Printf("Persistant store created in '%s'\n", db_name)
	
	//generate new keys and exit if flag set
	if *flagNewKeys {
		db.generateKeys(*flagKeyQty)
		db.close()
		return
	}
	
	//display keys and exit if flag set
	if *flagDisplayKeys {
		db.displayKeys()
		db.close()
		return
	}
	
	//exit if start flag missing
	if !*flagStart {
		log.Println("To start service, use '-start' flag")
		db.close()
		return
	}
	
	err = db.newBucket("SALE")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Wallet Address: %s\n", addr)
	service_address_without_amount := addr.Clone()
	service_address_without_amount.Arguments = expected_arguments[:len(expected_arguments)-1]
	log.Printf("Integrated address to activate '%s', (without hardcoded amount) service: \n%s\n", PLUGIN_NAME, service_address_without_amount.String())

	// service address can be created client side for now
	service_address := addr.Clone()
	service_address.Arguments = expected_arguments
	log.Printf("Integrated address to activate '%s', service: \n%s\n", PLUGIN_NAME, service_address.String())

	go processing_thread(db) // keep processing
	
	log.Println("Press ctrl-c to exit")
	<-done 
	log.Println("Exiting...")
	db.close()
	
}

func processing_thread(db DataBase) {
	finishedErrors = make(map[string]bool)
	var err error

	for { // currently we traverse entire history

		time.Sleep(time.Second)

		var transfers rpc.Get_Transfers_Result
		err = rpcClient.CallFor(&transfers, "GetTransfers", rpc.Get_Transfers_Params{In: true, DestinationPort: DEST_PORT})
		if err != nil {
			log.Printf("Could not obtain gettransfers from wallet err %s\n", err)
			continue
		}

		for _, e := range transfers.Entries {
			if e.Coinbase || !e.Incoming { // skip coinbase or outgoing, self generated transactions
				continue
			}

			// check whether the entry has been processed before, if yes skip it
			
			already_processed := db.read("SALE", e.TXID)
			if already_processed != "" {
				continue
			}
			
			// check whether this service should handle the transfer
			if !e.Payload_RPC.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) ||
				DEST_PORT != e.Payload_RPC.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64) { // this service is expecting value to be specfic
				continue

			}
			
			_, ok := finishedErrors[e.TXID] 
			if !ok {
				log.Printf("Tx should be processed %s\n", e.TXID)
			}
			
			if expected_arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) { // this service is expecting value to be specfic
				value_expected := expected_arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
				if e.Amount != value_expected { 
					_, ok := finishedErrors[e.TXID] 
					if !ok {
						log.Printf("User transferred %d, we were expecting %d. so we will not do anything\n", e.Amount, value_expected) // this is an unexpected situation
						finishedErrors[e.TXID] = true //don't display errors again for this tx
						//continue	
					}
					continue //do nothing, but don't repeat error message
					
					
				}
				// value received is what we are expecting, so time for response

				response[0].Value = e.SourcePort // source port now becomes destination port, similar to TCP
				
				//pick a key to sell
				key, err := db.pickKey()
				if err != nil {
					log.Println(err) //to do: proper error handling here, e.g. if keys have run out, refund money
					continue
				}
				
				response[2].Value = fmt.Sprintf("Key purchased to redeem for ETH: %s.",key)

				//_, err :=  response.CheckPack(transaction.PAYLOAD0_LIMIT)) //  we only have 144 bytes for RPC

				// sender of ping now becomes destination
				var str string
				tparams := rpc.Transfer_Params{Transfers: []rpc.Transfer{{Destination: e.Sender, Amount: uint64(1), Payload_RPC: response}}}
				err = rpcClient.CallFor(&str, "Transfer", tparams)
				if err != nil {
					log.Printf("Sending reply tx err %s\n", err)
					continue
				}
				
				//mark key as sold
				err = db.keySold(key)
				if err != nil {
					log.Println(err) 
					
				}
				
				err = db.write("SALE", e.TXID, "done")
				if err != nil {
					log.Printf("Error updating db to err %s\n", err)
				} else {
					log.Printf("Replied successfully with key %s",key)
				}

			}
		}

	}
}


//makes qty of key pairs, saves public keys to file for export to Eth SC, returns secret keys for DB
func getNewKeyPairs(qty int) (secret map[string]string, err error){
	secret = make(map[string]string)
	public := make([]string,0)
	
	for i := 0; i <qty; i++ {
		s, p := makeHashPair()
		secret[s] = "available"
		public = append(public,p)
	}
	
	filename := "public_keys-" + strconv.FormatInt(time.Now().UnixNano(),10) + ".txt" //use time to make unique filename
		
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
 
	if err != nil {
		return
	}
 
	datawriter := bufio.NewWriter(file)
 
	for _, data := range public {
		_, _ = datawriter.WriteString(data + "\n")
	}
 
	datawriter.Flush()
	file.Close()
	
	err = nil
	return
}

func makeHashPair() (secret, public string){
	//generate random secret key
	entropy := make([]byte, 128)
	_, _ = rand.Read(entropy) 	
	h := sha256.New()
	h.Write(entropy)
	s := h.Sum(nil)
	secret = hex.EncodeToString(s[:])
		
	//generate hash of secret key
	h2 := sha256.New()
	h2.Write(s)
	p := h2.Sum(nil)
	public = hex.EncodeToString(p[:])
		
	return
		
}

//open DB or create new DB
func openDB(db_name string) (DataBase, error) {
	var d DataBase
	db, err := bbolt.Open(db_name, 0600, nil)
	if err != nil {
		return d, err
	}
	d.db = db
	return d, nil
}


func (d *DataBase) makeNewKeyPairs(qty int) error {
	entries, err := getNewKeyPairs(qty)
	if err != nil {
		return err
	}
	err = d.writeMultiple("SECRET_KEYS", entries)
	if err != nil {
		return err
	}
	return nil
	
}

//create new bucket
func (d *DataBase) newBucket(name string) error {
	err := d.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})
	return err
}

//write single k / v pair to bucket
func (d *DataBase) write(bucket, key, value string) error {
	err := d.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		return b.Put([]byte(key), []byte(value))
	})
	return err
}

//read single value from bucket, returns "" if key not found
func (d *DataBase) read(bucket, key string) string {
	val := ""
	d.db.View(func(tx *bbolt.Tx) error {
		if b := tx.Bucket([]byte(bucket)); b != nil {
			if v := b.Get([]byte(key)); v != nil { // if existing in bucket
				val = string(v)
			}
		}
		return nil
	})
	
	return val
}


//picks an available key 
func (d *DataBase) pickKey() (string, error) {
	entries, err := d.getAll("SECRET_KEYS")
	if err != nil {
		return "", err
	}
	
	for k, v := range(entries) {
		if v == "available" {
			return k, nil
		}
	} 
	
	return "", errors.New("All keys have been sold")
}

//marks key as sold
func (d *DataBase) keySold(k string) error {
	err := d.write("SECRET_KEYS", k, "sold")
	return err
}


//return all k / v pairs from bucket
func (d *DataBase) getAll(bucket string) (map[string]string, error) {
	data := make(map[string]string)
	if err := d.db.View(func(tx *bbolt.Tx) error {
		if b := tx.Bucket([]byte(bucket)); b != nil {
			err := b.ForEach(func(k, v []byte) error {
				data[string(k)] = string(v)
				return nil
			})
			return err
		}
		return nil
	}); err != nil {
		return data, err
	}
			
	return data, nil
}

//write multiple k / v pairs to bucket
func (d *DataBase) writeMultiple(bucket string, entries map[string]string) error {
	err := d.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		for k, v := range(entries) {
			err := b.Put([]byte(k), []byte(v))
			if err != nil {
				return err
			}
		}
		return nil
		
	})
	return err
}

func (d *DataBase) close() {
	d.db.Close()
}

//Command line option - generate new key pairs
func (d *DataBase) generateKeys(qty int) {
	err := d.newBucket("SECRET_KEYS")
	if err != nil {
		log.Fatal(err)
	}
	
	err = d.makeNewKeyPairs(qty)
	if err != nil {
		log.Fatal(err)
	}
				
}

//Command line option - display keys in DB
func (d *DataBase) displayKeys() {
	m, err := d.getAll("SECRET_KEYS")
	if err != nil {
		log.Fatal(err)
	}
	
	for k, v := range(m) {
		log.Printf("k = %s, v = %s",k,v)
	}
		
}