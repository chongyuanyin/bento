package bundle

import (
	"errors"
	"fmt"

	"github.com/warpstreamlabs/bento/internal/component"
	"github.com/warpstreamlabs/bento/internal/component/input"
	"github.com/warpstreamlabs/bento/internal/component/output"
	"github.com/warpstreamlabs/bento/internal/docs"
)

type PreCheckerConstructor func(input.Config, output.Config, NewManagement) (component.Checkable, error)

type preCheckerSpec struct {
	constructor PreCheckerConstructor
	spec        docs.ComponentSpec
}

type PreCheckerSet struct {
	inputSpecs  map[string]preCheckerSpec
	outputSpecs map[string]preCheckerSpec
}

func (s *PreCheckerSet) Add(constructor PreCheckerConstructor, spec docs.ComponentSpec) error {
	switch spec.Type {
	case docs.TypeInput:
		if s.inputSpecs == nil {
			s.inputSpecs = map[string]preCheckerSpec{}
		}
		s.inputSpecs[spec.Name] = preCheckerSpec{
			constructor: constructor,
			spec:        spec,
		}
	case docs.TypeOutput:
		if s.outputSpecs == nil {
			s.outputSpecs = map[string]preCheckerSpec{}
		}
		s.outputSpecs[spec.Name] = preCheckerSpec{
			constructor: constructor,
			spec:        spec,
		}
	default:
		return fmt.Errorf("component type '%v' does not support pre-checkers", spec.Type)
	}

	return nil
}

func (s *PreCheckerSet) Init(inputConf *input.Config, outputConf *output.Config, mgr NewManagement) (component.Checkable, error) {
	if inputConf != nil && inputConf.Type != "" {
		spec, exists := s.inputSpecs[inputConf.Type]
		if exists {
			return spec.constructor(*inputConf, output.Config{}, mgr)
		}
	} else if outputConf != nil && outputConf.Type != "" {
		spec, exists := s.outputSpecs[outputConf.Type]
		if exists {
			return spec.constructor(input.Config{}, *outputConf, mgr)
		}
	}
	return nil, errors.New("no pre-checker found")
}

func (e *Environment) PreCheckerAdd(constructor PreCheckerConstructor, spec docs.ComponentSpec) error {
	return e.preChecker.Add(constructor, spec)
}
func (e *Environment) PreCheckerInit(inputConf *input.Config, outputConf *output.Config, mgr NewManagement) (component.Checkable, error) {
	return e.preChecker.Init(inputConf, outputConf, mgr)
}
