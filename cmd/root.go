package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "work",
	Short: "work-timer to log working-hours and sync them with your tickets/board",
	Long: `work-timer (short: work) helps to track working-hours.
TODO: fill this`,

	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(viper.GetString("test"))
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/.work-timer.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.AddConfigPath("$HOME/.config/")
		viper.AddConfigPath("config")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".work-timer")
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("could not find config file", err)
		} else {
			fmt.Println("error reading config file", err)
		}
	}
}
