package main

import (
	"github.com/QOSGroup/cassini/config"
	"github.com/QOSGroup/cassini/log"

	"context"

	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	mspclient "github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/resmgmt"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	fconfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	packager "github.com/hyperledger/fabric-sdk-go/pkg/fab/ccpackager/gopackager"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk/factory/defcore"
	"github.com/hyperledger/fabric-sdk-go/test/integration"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/common/cauthdsl"
)

const (
	//channelID      = "mychannel"
	//orgName        = "Org1"
	//orgAdmin       = "Admin"
	//ordererOrgName = "OrdererOrg"
	channelID      = "orgchannel"
	orgName        = "org1"
	orgAdmin       = "Admin"
	ordererOrgName = "OrdererOrg"
	ccID           = "example_cc_e2e"
)

//var ConfigTestFilename = "/root/go/pkg/mod/github.com/securekey/fabric-examples@v0.0.0-20190128203140-4d03d1c1e50f/fabric-cli/test/fixtures/config/config_test_local.yaml"
var ConfigTestFilename = "config_test_local.yaml"

var fabrictest = func(conf *config.Config) (cancel context.CancelFunc, err error) {

	log.Info("2.Starting fabrictest...")

	//var configOpt core.ConfigProvider
	configOpt := fconfig.FromFile(integration.GetConfigPath(ConfigTestFilename))
	sdkOpts := fabsdk.WithCorePkg(&CustomCryptoSuiteProviderFactory{})
	sdk, err := fabsdk.New(configOpt, sdkOpts)
	if err != nil {
		log.Errorf("Failed to create new SDK: %s", err)
	}
	defer sdk.Close()
	log.Info("3.fabsdk created")

	//clientContext allows creation of transactions using the supplied identity as the credential.
	clientContext := sdk.Context(fabsdk.WithUser(orgAdmin), fabsdk.WithOrg(ordererOrgName))

	resMgmtClient, err := resmgmt.New(clientContext)
	if err != nil {
		log.Errorf("Failed to create channel management client: %s", err)
	}
	log.Info("4.resMgmtClient created")

	//create channel
	mspClient, err := mspclient.New(sdk.Context(), mspclient.WithOrg(orgName))
	if err != nil {
		log.Error(err)
	}
	adminIdentity, err := mspClient.GetSigningIdentity(orgAdmin)
	if err != nil {
		log.Error(err)
	}
	req := resmgmt.SaveChannelRequest{ChannelID: channelID,
		ChannelConfigPath: integration.GetChannelConfigPath(channelID + ".tx"),
		SigningIdentities: []msp.SigningIdentity{adminIdentity}}
	txID, err := resMgmtClient.SaveChannel(req, resmgmt.WithRetry(retry.DefaultResMgmtOpts), resmgmt.WithOrdererEndpoint("orderer.example.com"))
	if err != nil {
		log.Errorf("create channel error:", err)
	} else {
		log.Infof("create channel txid:", txID)
	}

	//prepare context
	adminContext := sdk.Context(fabsdk.WithUser(orgAdmin), fabsdk.WithOrg(orgName))

	// Org resource management client
	orgResMgmt, err := resmgmt.New(adminContext)
	if err != nil {
		log.Errorf("Failed to create new resource management client: %s", err)
	}
	log.Info("4.orgResMgmt created")

	// Org peers join channel
	if err = orgResMgmt.JoinChannel(channelID, resmgmt.WithRetry(retry.DefaultResMgmtOpts), resmgmt.WithOrdererEndpoint("orderer.example.com")); err != nil {
		log.Errorf("Org peers failed to JoinChannel: %s", err)
	}
	log.Info("5.peers joined channel")

	createCC(orgResMgmt)

	//prepare channel client context using client context
	clientChannelContext := sdk.ChannelContext(channelID, fabsdk.WithUser("User1"), fabsdk.WithOrg(orgName))
	// Channel client is used to query and execute transactions (Org1 is default org)
	client, err := channel.New(clientChannelContext)
	if err != nil {
		log.Errorf("Failed to create new channel client: %s", err)
	}

	existingValue := queryCC(client)
	log.Infof("7.existingValue:", existingValue)

	return
}

func queryCC(client *channel.Client, targetEndpoints ...string) []byte {
	response, err := client.Query(channel.Request{ChaincodeID: ccID, Fcn: "invoke", Args: integration.ExampleCCDefaultQueryArgs()},
		channel.WithRetry(retry.DefaultChannelOpts),
		channel.WithTargetEndpoints(targetEndpoints...),
	)
	if err != nil {
		log.Errorf("Failed to query funds: %s", err)
	}
	return response.Payload
}

// CustomCryptoSuiteProviderFactory is will provide custom cryptosuite (bccsp.BCCSP)
type CustomCryptoSuiteProviderFactory struct {
	defcore.ProviderFactory
}

func createCC(orgResMgmt *resmgmt.Client) {
	ccPkg, err := packager.NewCCPackage("github.com/example_cc", integration.GetDeployPath())
	if err != nil {
		log.Error(err)
	}
	// Install example cc to org peers
	installCCReq := resmgmt.InstallCCRequest{Name: ccID, Path: "github.com/example_cc", Version: "0", Package: ccPkg}
	_, err = orgResMgmt.InstallCC(installCCReq, resmgmt.WithRetry(retry.DefaultResMgmtOpts))
	if err != nil {
		log.Error(err)
	}
	// Set up chaincode policy
	ccPolicy := cauthdsl.SignedByAnyMember([]string{"Org1MSP"})
	// Org resource manager will instantiate 'example_cc' on channel
	resp, err := orgResMgmt.InstantiateCC(
		channelID,
		resmgmt.InstantiateCCRequest{Name: ccID, Path: "github.com/example_cc", Version: "0", Args: integration.ExampleCCInitArgs(), Policy: ccPolicy},
		resmgmt.WithRetry(retry.DefaultResMgmtOpts),
	)
	if err != nil {
		log.Error(err)
	} else {
		log.Infof("6.createCC resp.TransactionID:", resp.TransactionID)
	}
}

func newInvokeAction() {
	//flags := &pflag.FlagSet{}
	/*chaincode invoke
	 --cid orgchannel
	 --ccid=examplecc
	 --args='{"Func":"move","Args":["A","B","1"]}'
	 --orgid org1
	 --base64
	 --config ../../test/fixtures/config/config_test_local.yaml

	chaincode id:examplecc
	configFile:../../test/fixtures/config/config_test_local.yaml
	args {"Func":"move","Args":["A","B","1"]} {}
	attempts 3 3
	backoff 1000 1000
	backofffactor 2 2
	base64 true false
	cacert
	ccid examplecc
	cid orgchannel
	concurrency 1 1
	config ../../test/fixtures/config/config_test_local.yaml
	format display display
	help false false
	iterations 1 1
	logging-level ERROR ERROR
	maxbackoff 5000 5000
	orgid org1
	payload false false
	peer
	pw
	selectprovider auto auto
	sleep 0 0
	timeout 5000 5000
	user
	verbose false false
	writer stdout stdout
	*/

}

//func invokechaincode() error {
//	//just test
//	fmt.Println("4.CryptoConfigPath:" + a.EndpointConfig().CryptoConfigPath())
//	channelClient, err := a.ChannelClient()
//	if err != nil {
//		return errors.Errorf("Error getting channel client: %v", err)
//	}
//
//	argsArray, err := action.ArgsArray()
//	if err != nil {
//		return err
//	}
//
//	executor := executor.NewConcurrent("Invoke Chaincode", cliconfig.Config().Concurrency())
//	executor.Start()
//	defer executor.Stop(true)
//
//	success := 0
//	var errs []error
//	var successDurations []time.Duration
//	var failDurations []time.Duration
//
//	var targets []fab.Peer
//	if len(cliconfig.Config().PeerURL()) > 0 || len(cliconfig.Config().OrgIDs()) > 0 {
//		targets = a.Peers()
//	}
//
//	var wg sync.WaitGroup
//	var mutex sync.RWMutex
//	var tasks []*invoketask.Task
//	var taskID int
//	for i := 0; i < cliconfig.Config().Iterations(); i++ {
//		for _, args := range argsArray {
//			taskID++
//			var startTime time.Time
//			task := invoketask.New(
//				strconv.Itoa(taskID), channelClient, targets,
//				cliconfig.Config().ChaincodeID(),
//				&args, executor,
//				retry.Opts{
//					Attempts:       cliconfig.Config().MaxAttempts(),
//					InitialBackoff: cliconfig.Config().InitialBackoff(),
//					MaxBackoff:     cliconfig.Config().MaxBackoff(),
//					BackoffFactor:  cliconfig.Config().BackoffFactor(),
//					RetryableCodes: retry.ChannelClientRetryableCodes,
//				},
//				cliconfig.Config().Verbose() || cliconfig.Config().Iterations() == 1,
//				cliconfig.Config().PrintPayloadOnly(), a.Printer(),
//
//				func() {
//					startTime = time.Now()
//				},
//				func(err error) {
//					duration := time.Since(startTime)
//					defer wg.Done()
//					mutex.Lock()
//					defer mutex.Unlock()
//					if err != nil {
//						errs = append(errs, err)
//						failDurations = append(failDurations, duration)
//					} else {
//						success++
//						successDurations = append(successDurations, duration)
//					}
//				})
//			tasks = append(tasks, task)
//		}
//	}
//
//	numInvocations := len(tasks)
//
//	wg.Add(numInvocations)
//
//	done := make(chan bool)
//	go func() {
//		ticker := time.NewTicker(10 * time.Second)
//		for {
//			select {
//			case <-ticker.C:
//				mutex.RLock()
//				if len(errs) > 0 {
//					fmt.Printf("*** %d failed invocation(s) out of %d\n", len(errs), numInvocations)
//				}
//				fmt.Printf("*** %d successfull invocation(s) out of %d\n", success, numInvocations)
//				mutex.RUnlock()
//			case <-done:
//				return
//			}
//		}
//	}()
//
//	startTime := time.Now()
//
//	for _, task := range tasks {
//		if err := executor.Submit(task); err != nil {
//			return errors.Errorf("error submitting task: %s", err)
//		}
//	}
//
//	// Wait for all tasks to complete
//	wg.Wait()
//	done <- true
//
//	duration := time.Now().Sub(startTime)
//
//	var allErrs []error
//	var attempts int
//	for _, task := range tasks {
//		attempts = attempts + task.Attempts()
//		if task.LastError() != nil {
//			allErrs = append(allErrs, task.LastError())
//		}
//	}
//
//	if len(errs) > 0 {
//		fmt.Printf("\n*** %d errors invoking chaincode:\n", len(errs))
//		for _, err := range errs {
//			fmt.Printf("%s\n", err)
//		}
//	} else if len(allErrs) > 0 {
//		fmt.Printf("\n*** %d transient errors invoking chaincode:\n", len(allErrs))
//		for _, err := range allErrs {
//			fmt.Printf("%s\n", err)
//		}
//	}
//
//	return nil
//}
