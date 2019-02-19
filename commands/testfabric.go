package commands

import (
	"github.com/QOSGroup/cassini/config"
	"github.com/spf13/cobra"
)

func addtestFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&config.GetConfig().ConfigFile, "config", "./config/config_fabric_test.yaml", "config file path")

	//cmd.Flags().StringVar(&config.GetConfig().LogConfigFile, "log", "./config/log.conf", "log config file path")

}

func NewTestCommand(run Runner, isKeepRunning bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "invoke fabric chaincode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commandRunner(run, isKeepRunning)
		},
	}

	addFlags(cmd)
	return cmd
}
