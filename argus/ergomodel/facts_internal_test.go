package ergomodel

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis"
)

func populated() []analysis.Fact {
	return []analysis.Fact{
		&ShapeFact{
			Aliasing: true, Opaque: true, WireOK: true, Message: true,
			Local: true, Allowed: true, Clone: true, Witness: "Items []string",
		},
		&SenderFact{Params: []int{1, 3}},
		&BlocksFact{Why: "mutex", Bounded: true, Timeout: 5 * time.Second},
		&SpawnsFact{},
		&UnrecoveredSpawnFact{},
		&RecoversFact{},
		&EscapesFact{Escapes: []Escape{{Result: 1, Field: "data"}}},
		&RepliesFact{},
		&FactoryFact{Behavior: "example.com/app.Worker"},
		&InitBudgetFact{Blocks: true, Why: "request", InnerTimeout: 7, Chain: "example.com/app.Worker.Init"},
		&InitSentinelFact{Sentinel: "gen.TerminateReasonNormal"},
		&RoundTripFact{Why: "a database round trip"},
		&NodeSenderFact{Params: []int{0, 2}, Method: "Send"},
		&FmtWrappedFact{},
		&ExternalRoundTripFact{Why: "a database round trip"},
	}
}

func TestFactsSurviveGobRoundTrip(t *testing.T) {
	for _, fact := range populated() {
		name := reflect.TypeOf(fact).String()

		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(fact); err != nil {
			t.Errorf("%s: encode: %s", name, err)
			continue
		}

		fresh := reflect.New(reflect.TypeOf(fact).Elem()).Interface()
		if err := gob.NewDecoder(&buf).Decode(fresh); err != nil {
			t.Errorf("%s: decode: %s", name, err)
			continue
		}
		if reflect.DeepEqual(fact, fresh) == false {
			t.Errorf("%s did not survive the round trip:\n got %+v\nwant %+v", name, fresh, fact)
		}
	}
}

func TestFactFieldsAreAllExported(t *testing.T) {
	var check func(t *testing.T, path string, rt reflect.Type, depth int)
	check = func(t *testing.T, path string, rt reflect.Type, depth int) {
		if depth > 4 {
			return
		}
		switch rt.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			check(t, path, rt.Elem(), depth+1)
		case reflect.Map:
			check(t, path+" key", rt.Key(), depth+1)
			check(t, path+" value", rt.Elem(), depth+1)
		case reflect.Struct:
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				if f.IsExported() == false {
					t.Errorf("%s.%s is unexported, so gob drops it and a dependent package reads a zero value",
						path, f.Name)
					continue
				}
				check(t, path+"."+f.Name, f.Type, depth+1)
			}
		}
	}
	for _, fact := range populated() {
		rt := reflect.TypeOf(fact)
		check(t, rt.String(), rt, 0)
	}
}

func TestPopulatedCoversEveryFactType(t *testing.T) {
	registered := map[reflect.Type]bool{}
	for _, f := range factTypes {
		registered[reflect.TypeOf(f)] = true
	}
	covered := map[reflect.Type]bool{}
	for _, f := range populated() {
		rt := reflect.TypeOf(f)
		covered[rt] = true
		if registered[rt] == false {
			t.Errorf("%s is exercised here but missing from FactTypes", rt)
		}
	}
	for rt := range registered {
		if covered[rt] == false {
			t.Errorf("%s is in FactTypes but not exercised by the round trip test", rt)
		}
	}
}
