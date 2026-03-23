package cli

import (
	"fmt"
	"os"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	var knowledge bool

	cmd := &cobra.Command{
		Use:   "validate <file.yaml>",
		Short: "Validate experiment or knowledge YAML without running",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if knowledge {
				return validateKnowledge(args[0])
			}
			return validateExperiment(args[0])
		},
	}

	cmd.Flags().BoolVar(&knowledge, "knowledge", false, "validate an OperatorKnowledge YAML file instead of an experiment")

	return cmd
}

func validateExperiment(path string) error {
	exp, err := experiment.Load(path)
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

	fmt.Printf("Experiment '%s' is valid.\n", exp.Name)
	return nil
}

func validateKnowledge(path string) error {
	k, err := model.LoadKnowledge(path)
	if err != nil {
		return fmt.Errorf("loading knowledge: %w", err)
	}

	errs := model.ValidateKnowledge(k)
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "Validation FAILED:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return fmt.Errorf("%d validation errors", len(errs))
	}

	fmt.Printf("Knowledge '%s' is valid.\n", k.Operator.Name)
	return nil
}
