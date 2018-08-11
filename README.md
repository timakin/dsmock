dsmock
====

dsmock is a fixture-injector for appengine datastore, based on YAML format fixtures.
Running unit tests with [aetest](https://cloud.google.com/appengine/docs/standard/go/tools/localunittesting/reference) is simple, but like as an integration test, when you try to test API handlers is very hard. One of the reasons is preparing mock data for datastore has no general way.
This package will help you to insert them without any painful steps.

## Install

`go get -u github.com/timakin/dsmock`

## Getting started

### fixtures
At first, you write the fixtures on YAML file.
This yaml must contains the information about your datastore entity with the keys: `schema` and `entities`.

```
scheme:
  kind: User
  key: ID

entities:
- ID: 1
  Name: John Doe
  Enabled: true
  CreatedAt: 1526477595
  UpdatedAt: 1526477595
- ID: 2
  Name: Jane Doe
  Enabled: true
  CreatedAt: 1526477595
  UpdatedAt: 1526477595
```

### Insert data

After you put fixtures on a directory, set up a pre-execution process to insert them into emulated datastore like this example. 

```
package testutils

import (
    ...
    "github.com/timakin/dsmock"
    ...
)

func Setup(t *testing.T) (context.Context, aetest.Instance, func()) {
	instance, ctx, err := testerator.SpinUp()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Insert fixtures
	fps := getFixturePaths()
	for _, fp := range fps {
		SetupFixtures(ctx, fp)
	}

	return ctx, instance, func() { testerator.SpinDown() }
}


func SetupFixtures(ctx context.Context, path string) error {
	return dsmock.InsertMockData(ctx, path)
}

func getFixturePaths() []string {
	datadir, _ := filepath.Abs("path/to/fixtures")
	return dirwalk(datadir)
}

func dirwalk(dir string) []string {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var paths []string
	for _, file := range files {
		if file.IsDir() {
			paths = append(paths, dirwalk(filepath.Join(dir, file.Name()))...)
			continue
		}
		paths = append(paths, filepath.Join(dir, file.Name()))
	}

	return paths
}
```

Then, call that setup func on each tests. Unless a volume of fixture is not huge, it works smoothly.  

```

func TestGetUsersHandler(t *testing.T) {
    ctx, instance, cleanup := testutils.Setup(t)
    defer cleanup()
	
    //
    // integration tests with fixtures.
    // 
}
```

License
The MIT License (MIT)

Copyright (c) 2018 Seiji Takahashi