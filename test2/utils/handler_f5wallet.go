package utils

import (
  // "time"
  "math/big"
  // "strings"
  "fmt"
  // "encoding/json"
  "errors"
  "strings"
  "github.com/ethereum/go-ethereum/crypto"
  	"github.com/ethereum/go-ethereum/common"
    _ "github.com/jinzhu/gorm/dialects/mysql"
      "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/accounts/abi/bind"
  "github.com/jinzhu/gorm"
  "encoding/hex"
  "test_eth/contracts/f5coin"
  "sync"
  "math"
  "strconv"
)

type F5WalletHandler struct {
    Client *RpcRouting
    Wallets []*WalletAccount
    ContractAddress common.Address
    Current int
    Mutex sync.Mutex
}
func stringTo32Byte(data string) [32]byte {
  //hexstring := hex.EncodeToString([]byte(data))
  var arr [32]byte
	copy(arr[:], data)
  return arr
}

func NewF5WalletHandler(contract_address string, client *RpcRouting)  *F5WalletHandler{
      contractAddress := common.HexToAddress(contract_address)
      wallHandler :=  &F5WalletHandler{
        Client: client,
        ContractAddress: contractAddress,
        Current: 0,
      }
      wallHandler.LoadAccountEth()
      wallHandler.AutoFillGas()
      // wallHandler.RegisterBatchEthToContract()
      return wallHandler
}
func (fw *F5WalletHandler) RegisterBatchEthToContract(requestTime int64) []string {
    ret := []string{}
    list := fw.GetAccountList()
    j := 0
    sublist :=  []common.Address{}
    for _,item := range list {
      if j == 0 {
        if len(sublist) > 0 {
          fmt.Println("Start register sublist")
          tx,err := fw.RegisterAccETH(requestTime,sublist)
          if err != nil {
             ret = append(ret, err.Error())
          } 	else {
             ret = append(ret, tx.Hash().Hex())
          }
          sublist = []common.Address{}
        }
      }
      sublist = append(sublist,item)
      j = ( j + 1 ) % 5
    }
    return ret
}
func (fw *F5WalletHandler) NewAccountEth() (string, error) {
      privateKey, err := crypto.GenerateKey()
      if err != nil {
        return "",err
      }
      address := crypto.PubkeyToAddress(privateKey.PublicKey)

      account := address.Hex()
      account = strings.TrimPrefix(account,"0x")
      account = strings.ToLower(account)

      priKey :=  hex.EncodeToString(crypto.FromECDSA(privateKey))

      new_account := &TokenAccount{
        Address: account,
        PrivateKey: priKey,
        Active: true,
      }

      fmt.Println("Update account to db ")
      db, err := gorm.Open("mysql", cfg.MysqlConnectionUrl())
      if cfg.Mysql.Debug {
         db.LogMode(true)
      }

      if err != nil {
        panic("failed to connect database: " + err.Error())
      }
      defer db.Close()
      //fmt.Println("Create new record")
      db.Create(new_account)

      fmt.Println("Update account to wallet ")
      wallet := WalletAccount{
        Routing: fw.Client,
        Address: account,
        PrivateKey: privateKey,
        Nonce: 0,
        Account: new_account,
        Active: false,
      }
      fw.Wallets = append(fw.Wallets,&wallet)
      return account, nil
}

func (fw *F5WalletHandler) GetAccountEthAddress(addr string) *WalletAccount {
    for _, wallet := range fw.Wallets {
       if wallet.Address == addr {
         return wallet
       }
    }
    return nil
}

func (fw *F5WalletHandler) GetAccountEth() *WalletAccount{
    fw.Mutex.Lock()
    defer fw.Mutex.Unlock()
    len := len(fw.Wallets)
    if len == 0 {
      return nil
    }
    if fw.Current >= len {
         fw.Current = fw.Current % len
    }
    wallet := fw.Wallets[fw.Current]
    fw.Current = fw.Current + 1
    return wallet
}

func (fw *F5WalletHandler) LoadAccountEth(){
  fmt.Println("Start load accounts from db to create wallets ")
  db, err := gorm.Open("mysql", cfg.MysqlConnectionUrl())
  if cfg.Mysql.Debug {
     db.LogMode(true)
  }

  if err != nil {
    panic("failed to connect database: " + err.Error())
  }
  defer db.Close()

  accounts := []TokenAccount{}

  if err := db.Where("active = ?", true).Find(&accounts).Error; err != nil {
    fmt.Println("Cannot get active Token Account in db: ",err)
    return
  }
  wallets := []*WalletAccount{}
  for _, account := range accounts {
      fmt.Println("Load wallet: ",account.Address)
      b, err := hex.DecodeString(account.PrivateKey)
     if err != nil {
          fmt.Println("invalid hex string: " + account.PrivateKey)
         continue
     }
      privateKey := crypto.ToECDSAUnsafe(b)
      wallet := WalletAccount{
        Routing: fw.Client,
        Address: account.Address,
        PrivateKey: privateKey,
        Active: true,
        Account: &account,
        Nonce: 0,
      }

      if cfg.Webserver.NonceMode == 2 {
          fmt.Println("Start sync nonce of ",account.Address)
          wallet.SyncNonce()
      }
      wallets = append(wallets,&wallet)
  }
  fmt.Println("End load accounts from db: ", len(wallets))
  fw.Mutex.Lock()
  defer fw.Mutex.Unlock()
  fw.Wallets = wallets
}
func  (fw *F5WalletHandler)  GetSummary() (int16,*big.Int, *big.Int, *big.Int,*big.Int)   {
      conn := fw.Client.GetConnection()
      instance, err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)

      n_account, err := instance.GetRegistedAccEthLength(&bind.CallOpts{})
      if err != nil {
        fmt.Println("Cannot Get Registed Acc Eth Length error: ",err)
        return 0, nil, nil, nil, nil
      }
      n_wallet, err := instance.GetStashNamesLenght(&bind.CallOpts{})
      if err != nil {
        fmt.Println("Cannot Get length of wallets, error: ",err)
        return n_account, nil, nil, nil, nil
      }
      n_credit, err := instance.GetCreditHistoryLength(&bind.CallOpts{})
      if err != nil {
        fmt.Println("Cannot Get length of credits, error: ",err)
        return n_account, n_wallet, nil, nil, nil
      }
      n_debit, err := instance.GetDebitHistoryLength(&bind.CallOpts{})
      if err != nil {
        fmt.Println("Cannot Get length of debit error: ",err)
        return n_account, n_wallet, n_credit, nil, nil
      }
      n_transfer, err := instance.GetTransferHistoryLength(&bind.CallOpts{})
      if err != nil {
        fmt.Println("Cannot Get length of transfer, error: ",err)
        return n_account, n_wallet, n_credit, n_debit, nil
      }
      return  n_account, n_wallet, n_credit, n_debit, n_transfer
}
func (fw *F5WalletHandler) CreateStash(requestTime int64, stashName string, typeStash int8) (*types.Transaction, error)  {
    retry := 0
    for retry <10 {
        account := fw.GetAccountEth()
        if account.IsAvailable() {
          conn := fw.Client.GetConnection()
          session, err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
          if err != nil {
              fmt.Println("Cannot find F5 contract")
              return nil,err
          }
          auth := account.NewTransactor()
          conn.Mux.Lock()
          defer  conn.Mux.Unlock()
          bs := stringTo32Byte(stashName)
          fmt.Println("Using: ", account, " to create Wallet: ",stashName, " len: ", len(bs) )
          tx,err := session.CreateStash(auth,bs, typeStash)
          if(err == nil){
            //Log transaction
            redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
            return tx, err
          }
        }
        retry = retry + 1
    }
    return nil, errors.New("Cannot find wallet in pool to create transaction")
}
func (fw *F5WalletHandler) GetBalance(stashName string) (*big.Int, error)  {
    fmt.Println("F5WalletHandler.GetBalance: Start get balance ")
    conn := fw.Client.GetConnection()
    conn.Mux.Lock()
    defer  conn.Mux.Unlock()
    session,err  := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
    if err != nil {
        fmt.Println("Cannot find F5 contract")
        return nil,err
    }
    fmt.Println("F5WalletHandler.GetBalance: call  GetBalance")
    return session.GetBalance(&bind.CallOpts{},stringTo32Byte(stashName))
}
// func (fw *F5WalletHandler) GetStateHistoryLength() (*big.Int, error)  {
//     conn := fw.Client.GetConnection()
//     conn.Mux.Lock()
//     defer  conn.Mux.Unlock()
//     session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
//     if err != nil {
//         fmt.Println("Cannot find F5 contract")
//         return nil,err
//     }
//     return session.GetStateHistoryLength(&bind.CallOpts{})
// }
func (fw *F5WalletHandler) SetState(requestTime int64, stashName string, stashState int8 ) (*types.Transaction, error)  {
  retry := 0
  for retry <10 {
      account := fw.GetAccountEth()
      if account.IsAvailable() {
          auth := account.NewTransactor()
          conn := fw.Client.GetConnection()
          session, err  := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
          if err != nil {
              fmt.Println("Cannot find F5 contract")
              return nil,err
          }
          conn.Mux.Lock()
          defer  conn.Mux.Unlock()
          tx,err := session.SetState(auth, stringTo32Byte(stashName),stashState)
          if(err == nil){
            //Log transaction
            redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
            return tx, err
          }
      }
        retry = retry + 1
  }
  return nil, errors.New("Cannot find wallet in pool to create transaction")
}

func (fw *F5WalletHandler) GetState(stashName string) (int8, error)  {
  conn := fw.Client.GetConnection()
  conn.Mux.Lock()
  defer  conn.Mux.Unlock()

  session, err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
  if err != nil {
      fmt.Println("Cannot find F5 contract")
      return 0,err
  }
  return session.GetState(&bind.CallOpts{},stringTo32Byte(stashName))

}
// func (fw *F5WalletHandler) GetRedeemHistoryLength() (*big.Int, error)  {
//     conn := fw.Client.GetConnection()
//     conn.Mux.Lock()
//     defer  conn.Mux.Unlock()
//
//     session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
//     if err != nil {
//         fmt.Println("Cannot find F5 contract")
//         return nil,err
//     }
//     return session.GetRedeemHistoryLength(&bind.CallOpts{})
// }
func (fw *F5WalletHandler) Debit(requestTime int64, txRef string, stashName string, amount *big.Int) (*types.Transaction, error) {
    retry := 0
    for retry <10 {
        account := fw.GetAccountEth()
        if account.IsAvailable() {
            auth := account.NewTransactor()
            conn := fw.Client.GetConnection()
            session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
            if err != nil {
                fmt.Println("Cannot find F5 contract")
                return nil,err
            }
            conn.Mux.Lock()
            defer  conn.Mux.Unlock()
            tx,err := session.Debit(auth, stringTo32Byte(txRef),stringTo32Byte(stashName),amount)
            if(err == nil){
              //Log transaction
              redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
              return tx, err
            }
        }
          retry = retry + 1
    }
    return nil, errors.New("Cannot find wallet in pool to create transaction")
}
// func (fw *F5WalletHandler) GetPledgeHistoryLength() (*big.Int, error)  {
//     conn := fw.Client.GetConnection()
//     conn.Mux.Lock()
//     defer  conn.Mux.Unlock()
//     session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
//     if err != nil {
//         fmt.Println("Cannot find F5 contract")
//         return nil,err
//     }
//     return session.GetPledgeHistoryLength(&bind.CallOpts{})
// }
func (fw *F5WalletHandler) Credit(requestTime int64, txRef string, stashName string, amount *big.Int) (*types.Transaction, error) {
  retry := 0
  for retry <10 {
      account := fw.GetAccountEth()
      if account.IsAvailable() {
          auth := account.NewTransactor()
          conn := fw.Client.GetConnection()
          session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)

          if err != nil {
              fmt.Println("Cannot find F5 contract")
              return nil,err
          }
          conn.Mux.Lock()
          defer  conn.Mux.Unlock()
          tx,err :=  session.Credit(auth, stringTo32Byte(txRef),stringTo32Byte(stashName),amount)
          if(err == nil){
            //Log transaction
            redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
            return tx, err
          }
      }
  }
  return nil, errors.New("Cannot find wallet in pool to create transaction")
}
func (fw *F5WalletHandler) GetTransferHistoryLength() (*big.Int, error)  {
  conn := fw.Client.GetConnection()
  conn.Mux.Lock()
  defer  conn.Mux.Unlock()
  session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
  if err != nil {
      fmt.Println("Cannot find F5 contract")
      return nil,err
  }
  return session.GetTransferHistoryLength(&bind.CallOpts{})

}
func (fw *F5WalletHandler) Transfer(requestTime int64, txRef string, sender string, receiver string, amount *big.Int, note string, txType int8) (*types.Transaction, error) {
  retry := 0
  for retry <10 {
      account := fw.GetAccountEth()
      if account.IsAvailable() {
          auth := account.NewTransactor()
          conn := fw.Client.GetConnection()
          session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
          if err != nil {
              fmt.Println("Cannot find F5 contract")
              return nil,err
          }
          conn.Mux.Lock()
          defer  conn.Mux.Unlock()
          tx, err :=  session.Transfer(auth, stringTo32Byte(txRef),stringTo32Byte(sender),stringTo32Byte(receiver),amount,note,txType)
          if(err == nil){
            //Log transaction
            redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
            return tx, err
          }
      }
        retry = retry + 1
  }
  return nil, errors.New("Cannot find wallet in pool to create transaction")
}
func (fw *F5WalletHandler) RegisterAccETH(requestTime int64, listAcc []common.Address) (*types.Transaction, error) {
  fmt.Println("Start RegisterAccETH")
  account := fw.GetAccountEthAddress(cfg.F5Contract.Owner)
  if account == nil {
     fmt.Println("Cannot find active account")
     return nil, errors.New("Cannot find bugdet account")
  }
  //if account.IsAvailable() {
      auth := account.NewTransactor()
      auth.GasLimit = 9000000
      conn := fw.Client.GetConnection()
      session,err := f5coin.NewBusiness(fw.ContractAddress,conn.Client)
      if err != nil {
          fmt.Println("Cannot find F5 contract")
          return nil,err
      }
      conn.Mux.Lock()
      defer  conn.Mux.Unlock()
      tx,err :=  session.RegisterAccETH(auth,listAcc)
      if(err == nil){
        //Log transaction
        redisCache.LogStart(tx.Hash().Hex(), 0, requestTime )
        return tx, err
      }
  // } else {
  //     fmt.Println("Account: ",account.Address," is unavailable ")
  // }
  fmt.Println("End RegisterAccETH: retry failed ")
  return nil, errors.New("Cannot find wallet in pool to create transaction")
}
func (fw *F5WalletHandler) GetAccountList() ([]common.Address) {
   fmt.Println("F5WalletHandler.GetAccountList: start read wallets")
   fw.Mutex.Lock()
   defer fw.Mutex.Unlock()
   accounts := []common.Address{}
   for _,wallet := range fw.Wallets {
       if wallet.Active {
         address := common.HexToAddress("0x"+wallet.Address)
         accounts = append(accounts,address)
       }
   }
   fmt.Println("F5WalletHandler.GetAccountList: end read wallets")
   return accounts
}

func (fw *F5WalletHandler) EthBalaneOf(account string) (*big.Float,error) {
  wallet := fw.GetAccountEthAddress(account)
  if wallet != nil {
      return wallet.EthBalaneOf()
  }
  return nil, errors.New("Cannot find account in system")
}
func (fw *F5WalletHandler) EthTransfer(from string,to string,amount string) (string,error) {
   wallet := fw.GetAccountEthAddress(from)

   fromAddress := common.HexToAddress("0x" + wallet.Address)
   nonce, err := wallet.Routing.PendingNonceAt(fromAddress)
   if err != nil {
     fmt.Println("Error in getting nonce ")
     return "", err
   }

   gLimit := cfg.Contract.GasLimit
   gPrice := cfg.Contract.GasPrice

   gasLimit := uint64(gLimit)
   gasPrice := new(big.Int)
   gasPrice, _ = gasPrice.SetString(gPrice, 10)

   toAddress := common.HexToAddress("0x" + to)

   eth_unit := big.NewFloat(math.Pow10(18))
   amount_value := new(big.Float)
   value, ok := amount_value.SetString(amount)

   if !ok {
        fmt.Println("SetString: error")
        return "", errors.New("convert amount error")
   }
   value = value.Mul(value,eth_unit)

   value_transfer := new(big.Int)
   value.Int(value_transfer)

   var data []byte
   rawTx := types.NewTransaction(nonce, toAddress, value_transfer, gasLimit, gasPrice, data)

   signer := types.FrontierSigner{}
   signature, err := crypto.Sign(signer.Hash(rawTx).Bytes(), wallet.PrivateKey)
   if err != nil {
     fmt.Println(" Cannot sign contract: ", err)
     return "",err
   }

   signedTx, err := rawTx.WithSignature(signer, signature)

   txhash := strings.TrimPrefix(signedTx.Hash().Hex(),"0x")
   err = wallet.Routing.SubmitTransaction(signedTx,nonce)

   return txhash, err
}

func (fw *F5WalletHandler) AutoFillGas() []string {
    fw.Mutex.Lock()
    defer fw.Mutex.Unlock()

    ret := []string{}
    for _, wallet := range fw.Wallets {
      bal, err := wallet.EthBalaneOf()
      if err != nil {
         fmt.Println("Cannot get wallet balance. Deactive wallet")
         wallet.Active = false
         continue
      }
      ba,_ := bal.Float64()
      if ba < 1000 {
         fmt.Println("Create transaction to fillGass from budget")
         var fill_account int = int(1000 - ba)

         txhash, err := fw.EthTransfer(cfg.F5Contract.EthBudget, wallet.Address,strconv.Itoa(fill_account))
         if err != nil {
           fmt.Println("Cannot fill more gas. Deactive wallet ")
           wallet.Active = false
           continue
         }
         fmt.Println("Fill Eth to account: ", wallet.Address, " transaction: ", txhash)
         ret = append(ret,txhash)
      } else {
         fmt.Println("Account: ", wallet.Address, " balance: ", ba)
      }
    }
    return ret
}
