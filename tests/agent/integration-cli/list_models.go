package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/gemyago/sonalmod/runtime/agent"
)

func runListModels(ctx context.Context, lister agent.ModelsLister, w io.Writer) error {
	if lister == nil {
		return errors.New("model listing is not available: runner has no models lister")
	}
	models, err := lister.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	return writeListModels(w, models)
}

func writeListModels(w io.Writer, models []agent.ModelInfo) error {
	slices.SortFunc(models, func(a, b agent.ModelInfo) int {
		if c := cmp.Compare(a.Provider, b.Provider); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	for _, m := range models {
		line := "* " + m.Provider + "/" + m.Name
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
