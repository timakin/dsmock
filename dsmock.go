package dsmock

import "context"

func InsertMockData(ctx context.Context, filename string) error {
	err := Upsert(ctx, filename)
	if err != nil {
		return err
	}
	return nil
}
