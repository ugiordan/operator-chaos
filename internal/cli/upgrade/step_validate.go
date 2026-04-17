package upgrade

import (
	"context"
	"fmt"
	"io"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// KnowledgeValidator is a function that validates knowledge models against a cluster.
type KnowledgeValidator func(ctx context.Context, models []*model.OperatorKnowledge) (pass, total int, err error)

// ValidateVersionExecutor runs version validation against a knowledge directory.
type ValidateVersionExecutor struct {
	validate KnowledgeValidator
}

func NewValidateVersionExecutor(validate KnowledgeValidator) *ValidateVersionExecutor {
	return &ValidateVersionExecutor{validate: validate}
}

func (e *ValidateVersionExecutor) Execute(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
	knowledgeDir := ResolveKnowledgeDir(step, pb)

	models, err := model.LoadKnowledgeDir(knowledgeDir)
	if err != nil {
		return fmt.Errorf("loading knowledge from %s: %w", knowledgeDir, err)
	}

	pass, total, err := e.validate(ctx, models)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	_, _ = fmt.Fprintf(out, "  %d/%d operators match expected state\n", pass, total)
	if pass < total {
		return fmt.Errorf("%d/%d operators failed validation", total-pass, total)
	}
	return nil
}
