package dsmock

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

const (
	// MaxBatchSize The number of entities per one multi upsert operation
	MaxBatchSize = 500
)

// Upsert entities form yaml file to datastore
func Upsert(ctx context.Context, filename string) error {
	batchSize := MaxBatchSize

	// Parser
	parser := NewYAMLParser()

	// Read from file
	if err := parser.ReadFile(filename); err != nil {
		return err
	}

	// Parse
	dsEntities, err := parser.Parse(ctx)
	if err != nil {
		return err
	}

	// Upsert to datastore
	allPage := int(math.Ceil(float64(len(*dsEntities)) / float64(batchSize)))
	for page := 0; page < allPage; page++ {

		from := page * batchSize
		to := (page + 1) * batchSize
		if to > len(*dsEntities) {
			to = len(*dsEntities)
		}

		// Upsert multi entities
		keys, src := getKeysValues(ctx, dsEntities, from, to)

		if _, err := datastore.PutMulti(ctx, keys, src); err != nil {
			if me, ok := err.(appengine.MultiError); ok {
				for i, e := range me {
					if e != nil {
						return fmt.Errorf("Upsert error(entity No.%v): %v\n", i+1, e)
					}
				}
			} else {
				return fmt.Errorf("Upsert error: %v\n", err)
			}
		} else {
			// core.Infof("%d entities ware upserted successfully.\n", len(keys))
		}
	}

	return nil
}

func getKeysValues(ctx context.Context, dsEntities *[]datastore.Entity, from, to int) (keys []*datastore.Key, values []interface{}) {

	// Prepare entities
	for _, e := range (*dsEntities)[from:to] {
		k := KeyToString(e.Key)
		if k == `""` {
			k = "(auto)"
		}

		keys = append(keys, e.Key)
		props := datastore.PropertyList(e.Properties)
		values = append(values, &props)
	}

	return
}
