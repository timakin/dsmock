package dsmock

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/appengine/datastore"
)

func ToString(value interface{}) string {
	switch t := value.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(value)
	}
}

func ToFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32, int, int32, int64:
		if v, ok := v.(float64); ok {
			return v, nil
		} else {
			return 0, fmt.Errorf("can not convert %v to float64", val)
		}
	case string:
		return strconv.ParseFloat(v, 64)
	}
	return 0, fmt.Errorf("can not convert %v to float64", val)
}

func KeyToString(k *datastore.Key) string {
	if k.Parent() == nil {
		if k.IntID() != 0 {
			return strconv.FormatInt(k.IntID(), 10)
		} else {
			return strconv.Quote(k.StringID())
		}
	}

	keys := make([]string, 0)

	for {
		var v string
		if k.IntID() != 0 {
			v = strconv.FormatInt(k.IntID(), 10)
		} else {
			v = strconv.Quote(k.StringID())
		}
		keys = append([]string{strconv.Quote(k.Kind()), v}, keys...)

		if k.Parent() == nil {
			return "[" + strings.Join(keys, ",") + "]"
		}

		k = k.Parent()
	}
}

func DecodeJSON(str string, value interface{}) error {
	return json.Unmarshal([]byte(str), value)
}
