package dsmock

import (
	"context"
	"errors"
	"io/ioutil"

	"google.golang.org/appengine/datastore"
	"gopkg.in/yaml.v2"
)

var (
	errNotDirectTypeValue = errors.New("NotDirectTypeValue")
)

type YAMLParser struct {
	parser *Parser
}

func NewYAMLParser() *YAMLParser {
	return &YAMLParser{
		parser: &Parser{
			&KindData{},
		},
	}
}

func (p *YAMLParser) ReadFile(filename string) error {
	source, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	d := &KindData{}
	if err = yaml.Unmarshal([]byte(source), d); err != nil {
		return err
	}
	p.parser.kindData = d
	return nil
}

func (p *YAMLParser) Parse(ctx context.Context) (*[]datastore.Entity, error) {
	if err := p.parser.Validate(); err != nil {
		return nil, err
	}

	d := *p.parser.kindData

	var res []datastore.Entity
	for _, e := range d.Entities {
		if entry, err := p.parser.ParseEntity(ctx, e); err != nil {
			return nil, err
		} else {
			res = append(res, entry)
		}
	}
	return &res, nil
}
