package yaml

import (
	"github.com/oarkflow/interpreter/pkg/config"
	yamlv3 "gopkg.in/yaml.v3"
)

func init() {
	parser := func(data []byte) (interface{}, error) {
		var v interface{}
		if err := yamlv3.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	config.RegisterFormatParser("yaml", parser)
	config.RegisterFormatParser("yml", parser)
}
