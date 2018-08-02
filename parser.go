package dsmock

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type FileParser interface {
	ReadFile(filename string) error
	Parse(kind string) (*[]datastore.Entity, error)
}

type KindData struct {
	Scheme   Scheme   `yaml:"scheme,omitempty"`
	Default  Default  `yaml:"default,omitempty"`
	Entities []Entity `yaml:"entities,omitempty"`
}

type Scheme struct {
	Namespace  string     `yaml:"namespace,omitempty"`
	Kind       string     `yaml:"kind,omitempty"`
	Key        string     `yaml:"key,omitempty"`
	TimeFormat string     `yaml:"time-format,omitempty"` // used for time.ParseInLocation()
	TimeLocale string     `yaml:"time-locale,omitempty"` // used for time.ParseInLocation()
	Properties Properties `yaml:"properties,omitempty"`
}

type Properties map[string]interface{}
type Default map[string]interface{}
type Entity map[string]interface{}

type Parser struct {
	kindData *KindData
}

func (p *Parser) Validate() error {
	if p.kindData.Scheme.Kind == "" {
		return errors.New("kind should be specified")
	}
	return nil
}

func (p *Parser) ParseEntity(ctx context.Context, entity Entity) (dsEntity datastore.Entity, err error) {
	d := *p.kindData

	var key *datastore.Key
	var props []datastore.Property

	// Values
	for name, val := range entity {
		if IsKeyValueName(name) {
			if key, err = p.parseKeyList(ctx, val); err != nil {
				return
			}

		} else {
			var prop *datastore.Property
			prop, err = p.parseProperty(ctx, name, val)
			if err != nil {
				return
			}
			props = append(props, *prop)
			if key == nil && name == p.kindData.Scheme.Key {
				if key, err = p.parseKeyList(ctx, prop.Value); err != nil {
					return
				}
			}
		}
	}

	// Default Values
	for name, val := range d.Default {
		if IsKeyValueName(name) {
			err = fmt.Errorf("%v can not be as default value", name)
			return
		}
		if _, ok := entity[name]; !ok {
			if prop, e := p.parseProperty(ctx, name, val); e != nil {
				err = e
				return
			} else {
				props = append(props, *prop)
			}
		}
	}

	if key == nil {
		key = p.getDSIncompleteKey(ctx, d.Scheme.Kind, nil)
	}

	return datastore.Entity{
		Key:        key,
		Properties: props,
	}, nil
}

func (p *Parser) parseKeyList(ctx context.Context, val interface{}) (key *datastore.Key, err error) {
	d := p.kindData

	if s, ok := val.(string); ok {
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			var arr []interface{}
			if err = DecodeJSON(s, &arr); err != nil {
				return
			} else {
				val = arr
			}
		}
	}

	keys, ok := val.([]interface{})
	if !ok {
		return p.parseKey(ctx, d.Scheme.Kind, val, nil)
	}

	var kind string
	for i, v := range keys {
		if i%2 == 0 {
			kind = ToString(v)
		} else {
			key, err = p.parseKey(ctx, kind, v, key)
			if err != nil {
				return
			}
		}
	}
	if len(keys)%2 != 0 {
		key = p.getDSIncompleteKey(ctx, kind, key)
	}
	return
}

func (p *Parser) parseKey(ctx context.Context, kind string, val interface{}, parent *datastore.Key) (key *datastore.Key, err error) {
	switch v := val.(type) {
	case string:
		key = p.getDSNamedKey(ctx, kind, v, parent)
	case int64:
		key = p.getDSIDKey(ctx, kind, v, parent)
	case int:
	case int32:
	case float32:
	case float64:
		key = p.getDSIDKey(ctx, kind, int64(v), parent)
	default:
		err = fmt.Errorf("key should be string or integer: %v, %v", reflect.TypeOf(val), v)
	}
	return
}

func (p *Parser) getDSIDKey(ctx context.Context, kind string, id int64, parent *datastore.Key) *datastore.Key {
	key := datastore.NewKey(ctx, kind, "", id, parent)
	return key
}

func (p *Parser) getDSNamedKey(ctx context.Context, kind string, nameKey string, parent *datastore.Key) *datastore.Key {
	key := datastore.NewKey(ctx, kind, nameKey, 0, parent)
	return key
}

func (p *Parser) getDSIncompleteKey(ctx context.Context, kind string, parent *datastore.Key) *datastore.Key {
	key := datastore.NewIncompleteKey(ctx, kind, parent)
	return key
}

func (p *Parser) parseProperty(ctx context.Context, name string, val interface{}) (*datastore.Property, error) {
	d := p.kindData

	spType, noIndex, err := p.getTypeInScheme(d.Scheme, name)
	if err != nil {
		return nil, err
	}

	var v interface{}
	if spType == "" {
		v, noIndex, err = p.parseValueAutomatically(ctx, val)

	} else if m, ok := val.(map[interface{}]interface{}); ok { // Check Directly Specified Types
		v, noIndex, err = p.parseDirectTypeValue(ctx, m)
		if err != nil {
			v, err = p.parseValueWithType(ctx, DatastoreType(spType), val)
		}

	} else {
		v, err = p.parseValueWithType(ctx, DatastoreType(spType), val)
	}

	if err != nil {
		return nil, err
	}

	return &datastore.Property{
		Name:    name,
		Value:   v,
		NoIndex: noIndex,
	}, nil
}

func (p *Parser) getTypeInScheme(scheme Scheme, name string) (string, bool, error) {
	for k, v := range scheme.Properties {
		if k == name {
			switch v := v.(type) {
			case string:
				return v, false, nil
			case nil:
				return "null", false, nil
			case []string:
				return v[0], IsNoIndex(v[1]), nil
			case []interface{}:
				return ToString(v[0]), IsNoIndex(ToString(v[1])), nil
			default:
				return "", false, fmt.Errorf("unsupported error:%v", v)
			}
		}
	}
	return "", false, nil
}

func (p *Parser) parseValueAutomatically(ctx context.Context, val interface{}) (value interface{}, noIndex bool, err error) {

	switch v := val.(type) {
	case string:
		var loc *time.Location
		loc, err = time.LoadLocation(p.kindData.Scheme.TimeLocale)
		if err != nil {
			return
		}
		if t, ok := p.parseTimestamp(v, loc); ok {
			value = t
		} else {
			value = v
		}
	case int:
		value = int64(v)
	case int64:
		value = v
	case float32:
		value = float64(v)
	case float64:
		value = v
	case bool:
		value = v
	case []interface{}:
		value, err = p.parseArray(ctx, v)
	case nil:
		value = v
	case map[interface{}]interface{}:
		value, noIndex, err = p.parseDirectTypeValue(ctx, v)
		if err == errNotDirectTypeValue {
			value, err = p.parseEmbed(ctx, v)
			noIndex = false
		}

	default:
		err = fmt.Errorf("can not parse value:%v", v)
	}
	return
}

func (p *Parser) parseDirectTypeValue(ctx context.Context, entry map[interface{}]interface{}) (value interface{}, noIndex bool, err error) {

	noIndexValue, ok := entry[KeywordNoIndex]
	if ok {
		if b, ok2 := noIndexValue.(bool); ok2 {
			noIndex = b
		}
	}

	for k, t := range keywordTypeMap {
		if v, ok := entry[k]; ok {
			value, err = p.parseValueWithType(ctx, t, v)
			return
		}
	}

	err = errNotDirectTypeValue
	return
}

func (p *Parser) parseValueWithType(ctx context.Context, spType DatastoreType, val interface{}) (value interface{}, err error) {
	d := p.kindData

	switch spType {
	case TypeString:
		value = ToString(val)

	case TypeDatetime:
		var loc *time.Location
		loc, err = time.LoadLocation(d.Scheme.TimeLocale)
		if err != nil {
			return
		}

		v := ToString(val)
		if v == "" {
			// break;

		} else if IsCurrentDatetime(v) {
			value = time.Now().In(loc)

		} else if d.Scheme.TimeFormat == "" {
			if t, ok := p.parseTimestamp(v, loc); ok {
				value = t
			} else {
				err = fmt.Errorf("can not parse '%v' as time.", v)
			}

		} else {
			if t, e := time.ParseInLocation(d.Scheme.TimeFormat, v, loc); e != nil {
				err = fmt.Errorf("can not parse '%v' as time.", e)
			} else {
				value = t
			}
		}

	case TypeInteger, TypeInt:
		var str = ToString(val)
		if str == "" {
			value = 0
		} else if num, e := strconv.ParseInt(str, 10, 64); e != nil {
			err = fmt.Errorf("can not parse '%v' as int. err:%v", str, e)
		} else {
			value = num
		}

	case TypeFloat:
		if num, ok := val.(float64); !ok {
			if num, err := strconv.ParseFloat(ToString(val), 64); err != nil {
				err = fmt.Errorf("can not parse '%v' as bool.", val)
			} else {
				value = num
			}
		} else {
			value = num
		}

	case TypeBoolean, TypeBool:
		if num, err := strconv.ParseBool(ToString(val)); err != nil {
			err = fmt.Errorf("can not parse '%v' as bool.", val)
		} else {
			value = num
		}

	case TypeNull, TypeNil:
		value = nil

	case TypeKey:
		value, err = p.parseKeyList(ctx, val)

	case TypeGeo:
		if value, err = p.parseGeoPoint(val); err != nil {
			return
		}

	case TypeArray:
		switch t := val.(type) {
		case []interface{}:
			value, err = p.parseArray(ctx, t)

		case string:
			var arr []interface{}
			if err = DecodeJSON(t, &arr); err != nil {
				return
			}
			value, err = p.parseArray(ctx, arr)

		default:
			err = fmt.Errorf("can not parse '%v' as array.", val)
		}

	case TypeBlob:
		blob, ok := val.(string)
		if !ok {
			err = fmt.Errorf("can not parse '%v' as base64 stings.", val)
			break
		}
		if b, e := base64.StdEncoding.DecodeString(blob); e != nil {
			err = fmt.Errorf("can not parse '%v' as base64 stings.(%v)", val, e)
		} else {
			value = b
		}

	case TypeEmbed:
		switch t := val.(type) {
		case map[interface{}]interface{}:
			value, err = p.parseEmbed(ctx, t)

		case string:
			if t == "" {
				value = nil
				break
			}
			var json map[string]interface{}
			if err = DecodeJSON(t, &json); err != nil {
				err = fmt.Errorf("can not parse '%v' as json.", t)
			} else {
				embed := make(map[interface{}]interface{})
				for k, v := range json {
					embed[k] = v
				}
				value, err = p.parseEmbed(ctx, embed)
			}

		default:
			err = fmt.Errorf("can not parse '%v' as embed.", val)
		}

	default:
		err = fmt.Errorf("property type '%v' is not supported.", spType)
	}

	return
}

func (p *Parser) parseArray(ctx context.Context, array []interface{}) ([]interface{}, error) {
	values := make([]interface{}, 0)

	for _, v := range array {
		value, _, err := p.parseValueAutomatically(ctx, v)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (p *Parser) parseEmbed(ctx context.Context, embed map[interface{}]interface{}) (*datastore.Entity, error) {
	props := make([]datastore.Property, 0)

	for name, v := range embed {

		value, _, err := p.parseValueAutomatically(ctx, v)
		if err != nil {
			return nil, err
		}
		props = append(props, datastore.Property{
			Name:  ToString(name),
			Value: value,
		})
	}

	return &datastore.Entity{
		Properties: props,
	}, nil
}

func (p *Parser) parseTimestamp(v interface{}, loc *time.Location) (time.Time, bool) {
	emptyTime := time.Time{}
	str, ok := v.(string)
	if !ok {
		return emptyTime, false
	}

	regxs := map[string]string{
		`^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$`: "2006-01-02",
		`^[0-9][0-9][0-9][0-9]` + // (year)
			`-[0-9][0-9]` + // (month)
			`-[0-9][0-9]` + // (day)
			`T[0-9][0-9]` + // (hour)
			`:[0-9][0-9]` + // (minute)
			`:[0-9][0-9]` + // (second)
			`Z|[-+][0-9][0-9]:[0-9][0-9]$`: time.RFC3339, // (time zone)
	}

	for regx, format := range regxs {
		if regexp.MustCompile(regx).MatchString(str) {
			t, err := time.ParseInLocation(format, str, loc) // (ymd)
			if err != nil {
				return emptyTime, false
			}
			return t, true
		}
	}
	return emptyTime, false
}

func (p *Parser) parseGeoPoint(val interface{}) (point appengine.GeoPoint, err error) {

	var geo []interface{}
	switch t := val.(type) {
	case []interface{}:
		geo = t

	case string:
		if err = DecodeJSON(t, &geo); err != nil {
			return
		}
	}

	if geo == nil {
		err = fmt.Errorf("can not parse '%v' as geo.", val)
		return
	}

	if len(geo) != 2 {
		err = fmt.Errorf("can not parse '%v' as geo point.", val)
		return
	}

	lat, err := ToFloat64(geo[0])
	if err != nil {
		err = fmt.Errorf("can not parse '%v' as geo point.", val)
		return
	}

	lng, err := ToFloat64(geo[1])
	if err != nil {
		err = fmt.Errorf("can not parse '%v' as geo point.", val)
		return
	}

	point = appengine.GeoPoint{
		Lat: lat,
		Lng: lng,
	}
	return
}
