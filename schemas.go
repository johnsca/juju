package main

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/bcsaller/jsonschema"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/rpcreflect"
)

func inspect(obj reflect.Type) {
	if obj == nil {
		return
	}
	s := jsonschema.ReflectFromType(obj)
	b, _ := json.MarshalIndent(s, "", "  ")
	fmt.Printf("%s\n", b)
}

func manual() {
	facades := common.Facades.List()
	for _, facade := range facades {
		version := facade.Versions[len(facade.Versions)-1]
		kind, err := common.Facades.GetType(facade.Name, version)
		if err != nil {
			continue
		}
		fmt.Printf("%s %d\n", facade.Name, version)
		objtype := rpcreflect.ObjTypeOf(kind)
		for _, m := range objtype.MethodNames() {
			method, _ := objtype.Method(m)
			fmt.Printf("\t%s: %v %v\n", m, method.Params, method.Result)
			inspect(method.Params)
			inspect(method.Result)
		}
	}
}

func main() {
	s := apiserver.DescribeFacadeSchemas()
	b, _ := json.MarshalIndent(s, "", "  ")
	fmt.Printf("%s\n", b)

}
