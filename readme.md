# ETH Seller - a Dero Service

This is a Dero service for the Stargate R2 testnet, written for the dARCH 2021 Event 0.5 competition: [https://forum.dero.io/t/darch-2021-event-0-5-services-only/1330](https://forum.dero.io/t/darch-2021-event-0-5-services-only/1330)

## Disclaimer

This service was written for the Dero Stargate testnet. It has not been extensively tested for security vulnerabilities, or peer reviewed. It may require modifications to function correctly on the main network, once the main network is released. Use at your own risk.

## Description

This service can operated by a person who wants to sell Ether for Dero automatically, without using an exchange. The service accepts a fixed amount of Dero (e.g. 1 Dero) and replies with a secret key (hash) that can be used to redeem a fixed amount of Ether (e.g. 0.1 Ether) from an Ethereum smart contract.

The mechanism works as follows: a series of key pairs are generated, secret and public. Each public key is a SHA-256 hash of the corresponding secret key. The public keys are stored in an Ethereum smart contract. The secret keys are stored in the service database. When the service receives the correct amount of Dero, it replies to the transaction with one of the secret keys. The buyer can then use this key to redeem a fixed amount of Ether from the Ethereum smart contract. The redemption process requires two stages, to prevent a third party from potentially grabbing the secret key in the Ethereum transaction pool and then paying a higher transaction fee to steal the Ether. Further details of this will be published in Round 2.

One significant limitation of this service is that users must trust the service provider: there is potential for the service provider to take the Dero and not provide the secret key. So it is not as secure as an atomic swap. In reality, this type of service provider would probably operate within a community where feedback from users can be posted, and transaction amounts would likely be small, to limit losses in the event of fraud.

The Ethereum smart contract will be developed and uploaded to this repo if this service makes it to Round 2 of the competition. 

## Compiling

All development was done on Windows 10 using Go 1.16.3 but it should compile with any recent version of Go. 

```
go get github.com/Lebowski1234/ethseller
```

## Usage

This is a command line utility. First, generate key pairs (100 in this example):

```
ethseller -newkeys -keyqty=100
```

The secret keys are stored in the database. The public keys will be saved to a .txt file in the same directory. 

Then open the Dero daemon and a dero wallet, with RPC enabled:

```
derod-windows-amd64  --testnet
```

```
dero-wallet-cli-windows-amd64 --rpc-server --rpc-bind=127.0.0.1:40403 --wallet-file wallet.db --testnet
```

Full details provided here: [https://forum.dero.io/t/dero-service-model/1309](https://forum.dero.io/t/dero-service-model/1309)

Then start the service:

```
ethseller -start
```


To view the list of secret keys and which one's have been sold by the service, stop the service (ctl+c) and use the following command:

```
ethseller -displaykeys
```

## Contact
I can be reached in the Dero project Discord channel (thedudelebowski#1775). 

If you found this code useful, any Dero donations are most welcome :) dERoVYHj6uBU4xjXVbn35ZiszZznGP2yZfnxqRSZZWvSbhjBaay8GC7cz8TTC54yfAChAjXCk6akeDh9Nmg8gEjm2G9Jb3wHg1
