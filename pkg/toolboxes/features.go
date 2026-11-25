package toolboxes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard/pkg/bgjobs"
)

type Feature struct {
	Name string // Features: foo:bar baz => foo
	Args string // Features: foo:bar baz => bar baz
}
type Features []Feature

func (f Features) Contains(name string) bool {
	return slices.Contains(f.Names(), name)
}

var (
	featureOptionNames = []string{"wait"}

	// featureRequirements must be set by llm if param required: TODO: use this func
	// other features are hidden/masked from llm tool spec and placed in cmdline args by [StaticExecFuncFS.ExecV1]
	//
	// Example 'wait':
	//
	// Param: wait - Wait for task to finish [boolean] [required]
	// Param: job_id - job id uuid given by manager [string] [required]
	// Feature: wait
	// Feature: job_id
	featureRequirements = map[string][]string{
		"wait": {"job_id"},
	}
)

func (f Features) NamesForWithout() []string {
	return slices.DeleteFunc(f.Names(), func(toolfeaturename string) bool {
		return !slices.Contains(featureOptionNames, toolfeaturename)
	})
}

func (f Features) Names() []string {
	if len(f) == 0 {
		return nil
	}
	a := make([]string, len(f))
	for i := range f {
		a[i] = f[i].Name
	}
	return a
}

func ArgsWithoutFeatures(params OrchParams, features Features, requiredParamNames []string) OrchParamsWrapper {
	featurenames := features.NamesForWithout()
	requiredParamNames = slices.DeleteFunc(requiredParamNames, func(v string) bool {
		return !slices.Contains(featureOptionNames, v)
	})
	//par := OrchParamsWrapper{Required: requiredParamNames, Properties: params.ToMap().Without(featurenames...)}
	return OrchParamsWrapper{
		Type:     "object",
		Required: requiredParamNames, Properties: params.ToMap().Without(featurenames...),
	}
}

func processFeature(ctx context.Context, argsmap map[string]json.RawMessage, v Feature) error {
	caller1, _ := mllm.CtxCaller(ctx)
	e, _ := mllm.CtxClient(ctx)
	if v.Name == "" {
		slog.Warn("broken feature no name")
		return nil
	}
	if _, ok := argsmap[v.Name]; ok {
		return fmt.Errorf("already had arg: %s", v.Name)
	}
	// if this parameter matches the feature name
	// add the feature as if the llm called with these args
	switch v.Name {
	case "database":
		argsmap["database"] = JSON(os.Getenv("ORCHARD_DB")) // TODO: use other path and move it after?
	case "thread_id":
		argsmap["thread_id"] = JSON(e.ThreadID)
	case "job_id":
		jobID := uuid.New()
		bgjobs.RegisterJob(jobID, caller1) // TODO: add ctx for info?
		argsmap["job_id"] = JSON(jobID.String())
	case "jobs_list":
		argsmap["jobs_list"] = JSON(bgjobs.ListJobs())
	case "wait":
		_ = featureRequirements // TODO:
		// no change, will be detected elsewhere
	}

	return nil
}
