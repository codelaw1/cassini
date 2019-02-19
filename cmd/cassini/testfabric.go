package main

import (
	"github.com/QOSGroup/cassini/config"
	"github.com/QOSGroup/cassini/log"

	"context"
	"fmt"
	//action "github.com/QOSGroup/cassini/adapter/action"
	cliconfig "github.com/QOSGroup/cassini/adapter/action/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/core"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/pkg/errors"
	action "github.com/securekey/fabric-examples/fabric-cli/cmd/fabric-cli/action"
	"github.com/securekey/fabric-examples/fabric-cli/cmd/fabric-cli/chaincode/invoketask"
	"github.com/securekey/fabric-examples/fabric-cli/cmd/fabric-cli/executor"
	"github.com/spf13/pflag"
	"strconv"
	"sync"
	"time"
)

var opts *options
var instance *CLIConfig

type options struct {
	certificate          string
	user                 string
	password             string
	loggingLevel         string
	orgIDsStr            string
	channelID            string
	chaincodeID          string
	chaincodePath        string
	chaincodeVersion     string
	peerURL              string
	ordererURL           string
	iterations           int
	sleepTime            int64
	configFile           string
	txFile               string
	txID                 string
	printFormat          string
	writer               string
	base64               bool
	args                 string
	chaincodeEvent       string
	seekType             string
	blockHash            string
	blockNum             uint64
	traverse             int
	chaincodePolicy      string
	collectionConfigFile string
	timeout              int64
	printPayloadOnly     bool
	concurrency          int
	maxAttempts          int
	initialBackoff       int64
	maxBackoff           int64
	backoffFactor        float64
	verbose              bool
	selectionProvider    string
	goPath               string
}

func init() {
	opts = &options{
		user:             "",
		password:         "",
		loggingLevel:     "ERROR",
		channelID:        "",
		orgIDsStr:        "",
		chaincodeVersion: "v0",
		iterations:       1,
		concurrency:      1,
		args:             getEmptyArgs(),
	}
}

// CLIConfig overrides certain configuration values with those supplied on the command-line
type CLIConfig struct {
	core.ConfigProvider
	//logger   *logging.Logger
	setFlags map[string]string
}

var fabrictest = func(conf *config.Config) (cancel context.CancelFunc, err error) {

	log.Info("2.Starting fabrictest...")

	a, err := newInvokeAction()
	defer a.Terminate()

	err = invokechaincode(a)
	if err != nil {
		log.Errorf("Error while running invokeAction: %v", err)
	}

	return
}

func newInvokeAction() (*action.Action, error) {
	flags := &pflag.FlagSet{}
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
	flags.StringVar(&opts.chaincodeID, cliconfig.ChaincodeIDFlag, "examplecc", "")
	flags.StringVar(&opts.channelID, cliconfig.ChannelIDFlag, "orgchannel", "")
	flags.StringVar(&opts.peerURL, cliconfig.PeerURLFlag, "", "")
	//flags.StringVar(&opts.configFile, cliconfig.ConfigFileFlag, "/root/go/pkg/mod/github.com/securekey/fabric-examples@v0.0.0-20190128203140-4d03d1c1e50f/fabric-cli/test/fixtures/config/config_test_local.yaml", "")

	flags.StringVar(&opts.args, cliconfig.ArgsFlag, "{\"Func\":\"move\",\"Args\":[\"A\",\"B\",\"1\"]}", "")
	flags.IntVar(&opts.maxAttempts, cliconfig.MaxAttemptsFlag, 3, "")
	flags.Float64Var(&opts.backoffFactor, cliconfig.BackoffFactorFlag, 2, "")
	flags.Int64Var(&opts.initialBackoff, cliconfig.InitialBackoffFlag, 1000, "")
	flags.Int64Var(&opts.maxBackoff, cliconfig.MaxBackoffFlag, 5000, "")
	flags.StringVar(&opts.selectionProvider, cliconfig.SelectionProviderFlag, "auto", "")

	flags.IntVar(&opts.iterations, cliconfig.IterationsFlag, 1, "")
	flags.Int64Var(&opts.sleepTime, cliconfig.SleepFlag, 0, "")
	flags.Int64Var(&opts.timeout, cliconfig.TimeoutFlag, 5000, "")
	flags.BoolVar(&opts.printPayloadOnly, cliconfig.PrintPayloadOnlyFlag, true, "")
	flags.IntVar(&opts.concurrency, cliconfig.ConcurrencyFlag, 1, "")

	a := action.Action{}
	err := a.Initialize(flags)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("3.CryptoConfigPath:" + a.EndpointConfig().CryptoConfigPath())
		//just test
		flags.VisitAll(func(f *pflag.Flag) {
			fmt.Println(f.Name, f.Value, f.DefValue)
		})
	}

	return &a, err

}

func invokechaincode(a *action.Action) error {
	channelClient, err := a.ChannelClient()
	if err != nil {
		return errors.Errorf("Error getting channel client: %v", err)
	}

	argsArray, err := action.ArgsArray()
	if err != nil {
		return err
	}

	executor := executor.NewConcurrent("Invoke Chaincode", cliconfig.Config().Concurrency())
	executor.Start()
	defer executor.Stop(true)

	success := 0
	var errs []error
	var successDurations []time.Duration
	var failDurations []time.Duration

	var targets []fab.Peer
	if len(cliconfig.Config().PeerURL()) > 0 || len(cliconfig.Config().OrgIDs()) > 0 {
		targets = a.Peers()
	}

	var wg sync.WaitGroup
	var mutex sync.RWMutex
	var tasks []*invoketask.Task
	var taskID int
	for i := 0; i < cliconfig.Config().Iterations(); i++ {
		for _, args := range argsArray {
			taskID++
			var startTime time.Time
			task := invoketask.New(
				strconv.Itoa(taskID), channelClient, targets,
				cliconfig.Config().ChaincodeID(),
				&args, executor,
				retry.Opts{
					Attempts:       cliconfig.Config().MaxAttempts(),
					InitialBackoff: cliconfig.Config().InitialBackoff(),
					MaxBackoff:     cliconfig.Config().MaxBackoff(),
					BackoffFactor:  cliconfig.Config().BackoffFactor(),
					RetryableCodes: retry.ChannelClientRetryableCodes,
				},
				cliconfig.Config().Verbose() || cliconfig.Config().Iterations() == 1,
				cliconfig.Config().PrintPayloadOnly(), a.Printer(),

				func() {
					startTime = time.Now()
				},
				func(err error) {
					duration := time.Since(startTime)
					defer wg.Done()
					mutex.Lock()
					defer mutex.Unlock()
					if err != nil {
						errs = append(errs, err)
						failDurations = append(failDurations, duration)
					} else {
						success++
						successDurations = append(successDurations, duration)
					}
				})
			tasks = append(tasks, task)
		}
	}

	numInvocations := len(tasks)

	wg.Add(numInvocations)

	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				mutex.RLock()
				if len(errs) > 0 {
					fmt.Printf("*** %d failed invocation(s) out of %d\n", len(errs), numInvocations)
				}
				fmt.Printf("*** %d successfull invocation(s) out of %d\n", success, numInvocations)
				mutex.RUnlock()
			case <-done:
				return
			}
		}
	}()

	startTime := time.Now()

	for _, task := range tasks {
		if err := executor.Submit(task); err != nil {
			return errors.Errorf("error submitting task: %s", err)
		}
	}

	// Wait for all tasks to complete
	wg.Wait()
	done <- true

	duration := time.Now().Sub(startTime)

	var allErrs []error
	var attempts int
	for _, task := range tasks {
		attempts = attempts + task.Attempts()
		if task.LastError() != nil {
			allErrs = append(allErrs, task.LastError())
		}
	}

	if len(errs) > 0 {
		fmt.Printf("\n*** %d errors invoking chaincode:\n", len(errs))
		for _, err := range errs {
			fmt.Printf("%s\n", err)
		}
	} else if len(allErrs) > 0 {
		fmt.Printf("\n*** %d transient errors invoking chaincode:\n", len(allErrs))
		for _, err := range allErrs {
			fmt.Printf("%s\n", err)
		}
	}

	if numInvocations > 1 {
		fmt.Printf("\n")
		fmt.Printf("*** ---------- Summary: ----------\n")
		fmt.Printf("***   - Invocations:     %d\n", numInvocations)
		fmt.Printf("***   - Concurrency:     %d\n", cliconfig.Config().Concurrency())
		fmt.Printf("***   - Successfull:     %d\n", success)
		fmt.Printf("***   - Total attempts:  %d\n", attempts)
		fmt.Printf("***   - Duration:        %2.2fs\n", duration.Seconds())
		fmt.Printf("***   - Rate:            %2.2f/s\n", float64(numInvocations)/duration.Seconds())
		//fmt.Printf("***   - Average:         %2.2fs\n", average(append(successDurations, failDurations...)))
		//fmt.Printf("***   - Average Success: %2.2fs\n", average(successDurations))
		//fmt.Printf("***   - Average Fail:    %2.2fs\n", average(failDurations))
		//fmt.Printf("***   - Min Success:     %2.2fs\n", min(successDurations))
		//fmt.Printf("***   - Max Success:     %2.2fs\n", max(successDurations))
		fmt.Printf("*** ------------------------------\n")
	}

	return nil
}

// Utility functions...

func getEmptyArgs() string {
	return "{}"
}

func getDefaultValueAndDescription(defaultValue string, defaultDescription string, overrides ...string) (value, description string) {
	if len(overrides) > 0 {
		value = overrides[0]
	} else {
		value = defaultValue
	}
	if len(overrides) > 1 {
		description = overrides[1]
	} else {
		description = defaultDescription
	}
	return value, description
}
