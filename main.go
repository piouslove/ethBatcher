// EthBatch project main.go
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"

	"database/sql"

	_ "github.com/go-sql-driver/mysql"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/olebedev/config"
	"github.com/onrik/ethrpc"
)

type Batcher struct {
	client       *ethclient.Client
	privateKey   *ecdsa.PrivateKey
	fromAddress  common.Address
	tokenAddress string
	gasLimit     uint64
	gasPrice     *big.Int
	nonce        uint64
	value        float64
}

var batcher *Batcher
var mysqlString string
var txCount int = 1

func (b *Batcher) syncGasPrice(url string) {
	rpc := ethrpc.New(url)
	gasPrice, err := rpc.EthGasPrice()
	if err != nil {
		log.Fatal(err)
	}
	b.gasPrice = &gasPrice
	fmt.Printf("Transaction GasPrice is %v \n", b.gasPrice)
}

func (b *Batcher) syncGasLimit() {
	if b.tokenAddress != "0x0" {
		b.gasLimit = uint64(70000)
	} else {
		b.gasLimit = uint64(30000)
	}
	fmt.Printf("Transaction GasLimit is %v \n", b.gasLimit)
}

func (b *Batcher) initNonce() {
	var err error
	b.nonce, err = b.client.PendingNonceAt(context.Background(), b.fromAddress)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Initial Nonce is %v \n", b.nonce)
}

func (b *Batcher) send(toAddress string, value float64) {
	var data []byte
	var amount *big.Int
	if b.tokenAddress != "0x0" {
		data = Hex2Bytes("0xa9059cbb")
		data = append(data, LeftPadBytes(Hex2Bytes(toAddress), 32)...)
		if b.value == float64(0) {
			tmp := big.NewFloat(value)
			tmp.Mul(tmp, big.NewFloat(1000000000000000000))
			amount, _ = tmp.Int(nil)
			data = append(data, LeftPadBytes(amount.Bytes(), 32)...)
		} else {
			tmp := big.NewFloat(b.value)
			tmp.Mul(tmp, big.NewFloat(1000000000000000000))
			amount, _ = tmp.Int(nil)
			data = append(data, LeftPadBytes(amount.Bytes(), 32)...)
		}
		toAddress = b.tokenAddress
		amount = big.NewInt(0)
		fmt.Printf("Transaction data is %s \n", data)
	} else {
		tmp := big.NewFloat(b.value)
		tmp.Mul(tmp, big.NewFloat(1000000000000000000))
		amount, _ = tmp.Int(nil)
		if b.value == float64(0) {
			tmp = big.NewFloat(value)
			tmp.Mul(tmp, big.NewFloat(1000000000000000000))
			amount, _ = tmp.Int(nil)
		}
		fmt.Printf("Transaction data is %s \n", data)
	}

	tx := types.NewTransaction(b.nonce, common.HexToAddress(toAddress), amount, b.gasLimit, b.gasPrice, data)
	signedTx, err := types.SignTx(tx, types.HomesteadSigner{}, b.privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = b.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Transaction NO.%d sent: %s \n", txCount, signedTx.Hash().Hex())
	b.nonce += 1
	txCount += 1
}

func NewBatcher(rpcURL, privateKeyHex, tokenAddress string, value float64) *Batcher {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatal(err)
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatal(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}
	// publicKeyECDSA := batcher.privateKey.PublicKey
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	return &Batcher{client, privateKey, fromAddress, tokenAddress, 0, big.NewInt(0), 0, value}
}

func init() {
	cfg, err := config.ParseJsonFile("./config.json")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cfg)

	privateKeyHex, err := cfg.String("privateKey")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(privateKeyHex)

	tokenAddress, err := cfg.String("tokenAddress")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(tokenAddress)

	mysqlURL, err := cfg.String("mysqlURL")
	mysqlUser, err := cfg.String("mysqlUser")
	mysqlPassword, err := cfg.String("mysqlPassword")
	rpcURL, err := cfg.String("rpcURL")
	if err != nil {
		log.Fatal(err)
	}

	value, err := cfg.Float64("value")
	if err != nil {
		log.Fatal(err)
	}

	if mysqlPassword == "" {
		mysqlString = mysqlUser + "@tcp(" + mysqlURL + ")/ethBatch"
	} else {
		mysqlString = mysqlUser + ":" + mysqlPassword + "@tcp(" + mysqlURL + ")/ethBatch"
	}
	fmt.Println(mysqlString)

	batcher = NewBatcher(rpcURL, privateKeyHex, tokenAddress, value)
	batcher.initNonce()
	batcher.syncGasLimit()
	batcher.syncGasPrice(rpcURL)
}

func main() {
	db, err := sql.Open("mysql", mysqlString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var toAddress string
	var id int
	var value float64

	for {
		if batcher.value != float64(0) {
			err := db.QueryRow("select id, toAddress from users where id > ?", id).Scan(
				&id, &toAddress)
			if err != nil {
				log.Fatal(err)
				return
			}
		} else {
			err := db.QueryRow("select id, toAddress, value from users where id > ?", id).Scan(
				&id, &toAddress, &value)
			if err != nil {
				log.Fatal(err)
				return
			}
		}
		batcher.send(toAddress, value)
		value = 0
	}
}

func Hex2Bytes(str string) []byte {
	if strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X") {
		str = str[2:]
	}

	if len(str)%2 == 1 {
		str = "0" + str
	}

	h, _ := hex.DecodeString(str)
	return h
}

func Int2Bytes(i uint64) []byte {
	return Hex2Bytes(Int2Hex(i))
}

func IntToBytes(n int) []byte {
	x := int32(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

// LeftPadBytes zero-pads slice to the left up to length l.
func LeftPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded[l-len(slice):], slice)

	return padded
}

func Int2Hex(number uint64) string {
	return fmt.Sprintf("%x", number)
}
