package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/urfave/cli/v2"
	"github.com/warpstreamlabs/bento/internal/bundle"
	"github.com/warpstreamlabs/bento/internal/cli/common"
	"github.com/warpstreamlabs/bento/internal/docs"
	"github.com/warpstreamlabs/bento/internal/stream"
	"gopkg.in/yaml.v3"
	// "github.com/warpstreamlabs/bento/internal/stream"
)

type ComponentConfig struct {
	Name        string                  `json:"name"`
	Type        docs.Type               `json:"type"`
	Summary     string                  `json:"summary,omitempty"`
	Description string                  `json:"description,omitempty"`
	Status      docs.Status             `json:"status,omitempty"`
	Categories  []string                `json:"categories"`
	Examples    []docs.AnnotatedExample `json:"examples,omitempty"`
	Version     string                  `json:"version,omitempty"`
	Fields      []ConfigField           `json:"fields"`
}

type ConfigField struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Type        docs.FieldType `json:"type"`
	Kind        docs.FieldKind `json:"kind"`
	Optional    bool           `json:"optional"`
	Secret      bool           `json:"secret"`
	Default     *FieldContent  `json:"default,omitempty"`
	Examples    []FieldContent `json:"examples,omitempty"`
	ChildFields []ConfigField  `json:"child_fields,omitempty"` // if Type is 'object', ChildFields will be populated
}

type FieldContent struct {
	StringContent string            `json:"string_content,omitempty"`
	ObjectContent map[string]string `json:"object_content,omitempty"`
	ArrayContent  []string          `json:"array_content,omitempty"`
}

func lookupConfig(c *cli.Context, cliOpts *common.CLIOpts) error {
	if c.Args().Len() < 2 {
		return fmt.Errorf("must specify component type and name")
	}
	typ := c.Args().Get(0)
	name := c.Args().Get(1)
	fmt.Println(typ, name)
	// conf := map[string]any{}
	// switch typ {
	// case "input", "output", "pipeline", "buffer":
	// 	conf[typ] = map[string]any{}
	// default:
	// 	err := fmt.Errorf("unsupported component: %v", typ)
	// 	return err
	// }

	// spec := stream.Spec()

	// conf, err := spec.AnyToMap(conf, docs.ToValueConfig{
	// 	FallbackToAny: true,
	// })
	// if err != nil {
	// 	return err
	// }

	// var cSpec docs.ComponentSpec
	// var exists bool
	// sanitConf := docs.NewSanitiseConfig(bundle.GlobalEnvironment)
	// for _, f := range spec {
	// 	if coreType, isCore := f.Type.IsCoreComponent(); isCore {
	// 		if f.Name == typ {
	// 			cSpec, exists = sanitConf.DocsProvider.GetDocs(name, coreType)
	// 			if !exists {
	// 				return fmt.Errorf("failed to obtain docs for %v type %v", coreType, name)
	// 			}
	// 			break
	// 		}
	// 	}
	// }

	// conf := ComponentConfig{
	// 	Name:        cSpec.Name,
	// 	Type:        cSpec.Type,
	// 	Summary:     cSpec.Summary,
	// 	Description: cSpec.Description,
	// 	Status:      cSpec.Status,
	// 	Categories:  cSpec.Categories,
	// 	Examples:    cSpec.Examples,
	// 	Version:     cSpec.Version,
	// }
	// fields, err := lookupChildren(cSpec.Config.Children)
	// if err != nil {
	// 	return err
	// }
	// conf.Fields = fields

	// fmt.Println("==========")
	// fmt.Println(conf)

	// filter := func(spec docs.FieldSpec, _ any) bool {
	// 	if _, isCore := spec.Type.IsCoreComponent(); isCore {
	// 		return spec.Name == typ
	// 	}
	// 	return false
	// }

	// conf, err := spec.AnyToMap(conf, docs.ToValueConfig{
	// 	FallbackToAny: true,
	// })
	// if err != nil {
	// 	return err
	// }

	// sanitConf := docs.NewSanitiseConfig(bundle.GlobalEnvironment)
	// sanitConf.RemoveTypeField = true
	// sanitConf.RemoveDeprecated = true
	// sanitConf.ForExample = true
	// sanitConf.Filter = filter

	// sanitConf.DocsProvider.GetDocs(name, typ)

	switch c.String("format") {
	case "json":
		jsonStr, err := formatJson(typ, name)
		if err != nil {
			return err
		}
		fmt.Println(jsonStr)
	case "yaml":
		yamlStr, err := formatYaml(typ, name)
		if err != nil {
			return err
		}
		fmt.Println(yamlStr)
	}

	return nil
}

func formatJson(typ, name string) (string, error) {
	spec := stream.Spec()

	var cSpec docs.ComponentSpec
	var exists bool
	sanitConf := docs.NewSanitiseConfig(bundle.GlobalEnvironment)
	for _, f := range spec {
		if coreType, isCore := f.Type.IsCoreComponent(); isCore {
			if f.Name == typ {
				cSpec, exists = sanitConf.DocsProvider.GetDocs(name, coreType)
				if !exists {
					return "", fmt.Errorf("failed to obtain docs for %v type %v", coreType, name)
				}
				break
			}
		}
	}

	conf := ComponentConfig{
		Name:        cSpec.Name,
		Type:        cSpec.Type,
		Summary:     cSpec.Summary,
		Description: cSpec.Description,
		Status:      cSpec.Status,
		Categories:  cSpec.Categories,
		Examples:    cSpec.Examples,
		Version:     cSpec.Version,
	}
	fields, err := lookupChildren(cSpec.Config.Children)
	if err != nil {
		return "", err
	}
	conf.Fields = fields

	jsonBytes, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func formatYaml(typ, name string) (string, error) {
	conf := map[string]any{}
	switch typ {
	case "input":
		if _, exists := bundle.AllInputs.DocsFor(name); exists {
			conf[typ] = map[string]any{
				"type": name,
			}
		} else {
			return "", fmt.Errorf("unrecognised input type '%v'", name)
		}
	case "output":
		if _, exists := bundle.AllOutputs.DocsFor(name); exists {
			conf[typ] = map[string]any{
				"type": name,
			}
		} else {
			return "", fmt.Errorf("unrecognised output type '%v'", name)
		}
	default:
		err := fmt.Errorf("unsupported component: %v", typ)
		return "", err
	}

	spec := stream.Spec()
	// filter := func(spec docs.FieldSpec, _ any) bool {
	// 	if _, isCore := spec.Type.IsCoreComponent(); isCore {
	// 		return spec.Name == typ
	// 	}
	// 	return false
	// }
	filter := func(spec docs.FieldSpec, _ any) bool {
		return !spec.IsAdvanced
	}
	fullConf, err := spec.AnyToMap(conf, docs.ToValueConfig{
		FallbackToAny: true,
	})
	if err != nil {
		return "", err
	}
	conf = map[string]any{}
	for k, v := range fullConf {
		if k == typ {
			conf[k] = v
			break
		}
	}

	var node yaml.Node
	err = node.Encode(conf)
	if err != nil {
		return "", err
	}
	sanitConf := docs.NewSanitiseConfig(bundle.GlobalEnvironment)
	sanitConf.RemoveTypeField = true
	sanitConf.RemoveDeprecated = true
	sanitConf.ForExample = true
	sanitConf.Filter = filter

	err = spec.SanitiseYAML(&node, sanitConf)
	if err != nil {
		return "", err
	}
	var configYAML []byte
	configYAML, err = docs.MarshalYAML(node)
	if err != nil {
		return "", err
	}
	return string(configYAML), nil
}

func lookupChildren(children docs.FieldSpecs) ([]ConfigField, error) {
	fields := []ConfigField{}
	for _, f := range children {
		field := ConfigField{
			Name:        f.Name,
			Description: f.Description,
			Type:        f.Type,
			Kind:        f.Kind,
			Optional:    f.IsOptional,
			Secret:      f.IsSecret,
		}

		if f.Type == docs.FieldTypeObject {
			childFields, err := lookupChildren(f.Children)
			if err != nil {
				return nil, err
			}
			field.ChildFields = childFields
		} else {
			defaultContent := FieldContent{}
			exampleContent := []FieldContent{}
			var val *any
			var examples []any

			if f.Default != nil {
				val = f.Default
			}
			if f.Examples != nil {
				examples = f.Examples
			}
			switch f.Kind {
			case docs.KindScalar:
				if val != nil {
					str, err := getScalarValue(*val, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					defaultContent.StringContent = str
				}
				for _, e := range examples {
					str, err := getScalarValue(e, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					exampleContent = append(exampleContent, FieldContent{
						StringContent: str,
					})
				}
			case docs.KindArray:
				if val != nil {
					arr, err := getArrayValue(*val, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					defaultContent.ArrayContent = arr
				}
				for _, e := range examples {
					arr, err := getArrayValue(e, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					exampleContent = append(exampleContent, FieldContent{
						ArrayContent: arr,
					})
				}
			case docs.KindMap:
				if val != nil {
					mp, err := getMapValue(*val, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					defaultContent.ObjectContent = mp
				}
				for _, e := range examples {
					mp, err := getMapValue(e, f.Type, f.Name)
					if err != nil {
						return nil, err
					}
					exampleContent = append(exampleContent, FieldContent{
						ObjectContent: mp,
					})
				}
			}
			field.Default = &defaultContent
			field.Examples = exampleContent
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func getScalarValue(val any, typ docs.FieldType, fieldName string) (string, error) {
	errMsg := fmt.Sprintf("failed to get string value from scalar for field %s", fieldName)
	switch typ {
	case docs.FieldTypeString:
		if str, ok := (val).(string); ok {
			return str, nil
		} else {
			return "", errors.New(errMsg)
		}
	case docs.FieldTypeInt:
		if intVal, ok := (val).(int); ok {
			return strconv.Itoa(int(intVal)), nil
		} else {
			return "", errors.New(errMsg)
		}
	case docs.FieldTypeFloat:
		if floatVal, ok := (val).(float64); ok {
			return strconv.FormatFloat(floatVal, 'f', -1, 64), nil
		} else {
			return "", errors.New(errMsg)
		}
	case docs.FieldTypeBool:
		if boolVal, ok := (val).(bool); ok {
			if boolVal {
				return "true", nil
			}
			return "false", nil
		} else {
			return "", errors.New(errMsg)
		}
	case docs.FieldTypeUnknown:
		if str, ok := (val).(string); ok {
			return str, nil
		} else {
			return "", errors.New(errMsg)
		}
	default:
		return "", fmt.Errorf("invalid field type %v", typ)
	}
}

func getArrayValue(val any, typ docs.FieldType, fieldName string) ([]string, error) {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Array || rv.Kind() == reflect.Slice {
		arr := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			str, err := getScalarValue(rv.Index(i).Interface(), typ, fieldName)
			if err != nil {
				return nil, err
			}
			arr[i] = str
		}
		return arr, nil
	} else {
		return nil, errors.New("failed to get array value")
	}
}

func getMapValue(val any, typ docs.FieldType, fieldName string) (map[string]string, error) {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Map {
		mp := make(map[string]string)
		for _, k := range rv.MapKeys() {
			str, err := getScalarValue(rv.MapIndex(k).Interface(), typ, fieldName)
			if err != nil {
				return nil, err
			}
			mp[k.String()] = str
		}
		return mp, nil
	} else {
		return nil, errors.New("failed to get map value")
	}
}

func lookupCliCommand(cliOpts *common.CLIOpts) *cli.Command {
	return &cli.Command{
		Name:  "lookup",
		Usage: cliOpts.ExecTemplate("Lookup config details for a component"),
		Description: cliOpts.ExecTemplate(`
Prints config details of an input or output component. Component type and name should be specified:

  {{.BinaryName}} lookup --format json input stdin
  {{.BinaryName}} lookup output stdout`)[1:],
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Value: "json",
				Usage: "Print the component config details in a specific format. Options are json or yaml",
			},
		},
		Action: func(c *cli.Context) error { //TODO
			err := lookupConfig(c, cliOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Lookup config error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}
}
