package cli

import (
	"fmt"
	"os"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [experiment.yaml]",
		Short: "Validate experiment YAML without running",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := experiment.Load(args[0])
			if err != nil {
				return fmt.Errorf("loading experiment: %w", err)
			}

			errs := experiment.Validate(exp)
			if len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "Validation FAILED:")
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
				return fmt.Errorf("%d validation errors", len(errs))
			}

			fmt.Printf("Experiment '%s' is valid.\n", exp.Metadata.Name)
			return nil
		},
	}
}
