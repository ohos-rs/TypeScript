// Package etstest supplies source-owned compiler configuration to tests, not
// replacement SDK declarations. ets.json is compilerOptions.ets extracted from
// ets2bundle compiler/tsconfig.json (70818d757b521238fcd1f680940c0c76356b7fc8).
package etstest

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/microsoft/TypeScript/tsc/internal/core"
)

//go:embed ets.json
var source []byte

var Options = sync.OnceValue(func() core.EtsOptions {
	var options core.EtsOptions
	if err := json.Unmarshal(source, &options); err != nil {
		panic(err)
	}
	return options
})
