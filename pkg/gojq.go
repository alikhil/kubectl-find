package pkg

import (
	"fmt"

	"github.com/itchyny/gojq"
)

type Person struct {
	Name string
	Age  int
}

func MatchesWithGoJQ(obj interface{}, expr *gojq.Query) (bool, error) {
	iter := expr.Run(obj)
	// jq expressions can return multiple values; treat first truthy as match
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return false, fmt.Errorf("jq evaluation error: %w", err)
		}
		// consider boolean true or non-empty value as match
		if b, isBool := v.(bool); isBool && b {
			return true, nil
		}
		if v != nil {
			return true, nil
		}
	}
	return false, nil
}

func PrepareQuery(expr string) (*gojq.Query, error) {
	expr = fmt.Sprintf("[.] | .[] | select ( %s ) | length > 0", expr)
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse jq expression: %w", err)
	}
	return query, nil
}

func MustPrepareQuery(expr string) *gojq.Query {
	query, err := PrepareQuery(expr)
	if err != nil {
		panic(err)
	}
	return query
}
