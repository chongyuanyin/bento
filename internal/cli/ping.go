package cli

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/warpstreamlabs/bento/internal/bundle"
	"github.com/warpstreamlabs/bento/internal/cli/common"
	"github.com/warpstreamlabs/bento/internal/component"
	"github.com/warpstreamlabs/bento/internal/component/input"
	"github.com/warpstreamlabs/bento/internal/component/output"
	"github.com/warpstreamlabs/bento/internal/manager"
)

func ping(c *cli.Context, cliOpts *common.CLIOpts) error {
	if c.Args().Len() > 0 {
		if c.Args().Len() > 1 {
			fmt.Fprintln(os.Stderr, "A maximum of one config must be specified with the ping command")
			os.Exit(1)
		}
		_ = c.Set("config", c.Args().First())
	}
	_, _, confReader := common.ReadConfig(c, cliOpts, false, false)

	conf, pConf, _, _, err := confReader.Read()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration file read error: %v\n", err)
		return err
	}
	defer func() {
		_ = confReader.Close(c.Context)
	}()
	// fmt.Println(conf)

	logger, err := common.CreateLogger(c, cliOpts, conf, false)
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return err
	}

	mgrOpts := []manager.OptFunc{
		manager.OptSetEngineVersion(cliOpts.Version),
		manager.OptSetLogger(logger),
		manager.OptSetStreamsMode(false),
	}
	mgr, err := manager.New(conf.ResourceConfig, mgrOpts...)
	if err != nil {
		err = fmt.Errorf("failed to initialise resources: %w", err)
		return err
	}

	if err := cliOpts.OnManagerInitialised(mgr, pConf); err != nil {
		logger.Error(err.Error())
		return err
	}

	checker, err := initPreChecker(mgr, &conf.Input, &conf.Output)
	if err != nil {
		logger.Error(err.Error())
		return err
	}
	if checker != nil {
		err := checker.Check()
		if err != nil {
			logger.Error(err.Error())
			return err
		}
	}

	return nil
}

func initPreChecker(mgr bundle.NewManagement, inputConf *input.Config, outputConf *output.Config) (component.Checkable, error) {
	return mgr.Environment().PreCheckerInit(inputConf, outputConf, mgr)
}

func pingCliCommand(cliOpts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "ping",
		Usage: cliOpts.ExecTemplate("Verify the connectivity of the input or output component"),
		Description: cliOpts.ExecTemplate(`
Ping according to the {{.ProductName}} config.

  {{.BinaryName}} ping ./foo.yaml`)[1:],
		Action: func(c *cli.Context) error {
			err := ping(c, cliOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Ping error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}
}
