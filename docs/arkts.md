# ArkTS 1.1 / ArkUI

The `ohos` branch adds an ArkUI frontend to the Go TypeScript compiler. `.ets`
and `.d.ets` select `ScriptKindETS`. `.ts` and `.tsx` retain their existing grammar;
static ETS / ArkTS 1.2 is not enabled.

## Using the compiler

Run the compiler against an ArkUI project with its actual OpenHarmony SDK type
declarations available through `include`, `files`, or package imports:

```json
{
    "compilerOptions": {
        "target": "es2020",
        "module": "esnext",
        "moduleResolution": "bundler",
        "lib": ["es2020"],
        "strict": true,
        "noEmit": true,
        "allowImportingTsExtensions": true
    },
    "include": ["src/**/*.ets", "sdk/**/*.d.ets"]
}
```

```sh
go run ./tsc/cmd/tsc -p /path/to/project/tsconfig.json
```

Use the SDK declarations matching your target device/API level. Browser `dom`
declarations can conflict with ArkUI names such as `Text`; they are deliberately
absent from the example. The repository does not supply replacement SDK types.

For declarations, replace `noEmit` with `declaration` and `emitDeclarationOnly`.
Outputs use `.d.ets` and preserve `struct` declarations. JavaScript emission of
ArkUI syntax produces diagnostic TS100069 and skips that JavaScript file. Runtime
lowering, state management and application packaging remain the SDK compiler's
responsibility. Ordinary ETS files containing only TypeScript syntax can still
use the regular JavaScript emitter.

## Frontend behavior

- Project discovery, extensionless imports, explicit ETS imports, directory
  indexes and declaration lookup recognize ETS extensions. Existing TS/TSX/DTS
  resolution candidates retain their precedence.
- `struct` is contextual and accepts members, generics, exports and decorators.
  Its members participate in normal binding, name lookup and type checking.
- Render syntax is enabled in struct `build()` methods and `@Builder` /
  `@LocalBuilder` bodies. PascalCase component calls accept trailing content
  blocks and chained attributes. Namespaced component names are also accepted.
- `ForEach` / `LazyForEach` callbacks and `Repeat().each()` / `.template()`
  callbacks can contain render syntax. Ordinary nested functions and event
  callbacks do not inherit it.
- `@Styles`, `@Extend` and `@AnimatableExtend` bodies accept leading-dot
  attributes. `stateStyles` accepts attribute blocks. When SDK types are present,
  attributes are checked against `CommonAttribute` or the component's
  `<Name>Attribute` type. Standalone style bodies fall back to `any` when these
  SDK declarations are unavailable.
- Custom struct calls use a property-bag signature, with contextual typing,
  excess-property checks, and `@Require` fields. SDK-managed properties such as
  `@Prop`, `@Link` and `@BuilderParam` need not be initialized by a constructor.
- Render-body binding references (`$property`, `$$variable`, `$$this.property`)
  resolve their underlying symbols/types. Declared dollar-prefixed names such as
  `$r` take precedence over the binding shorthand.
- Recognized ArkUI compiler decorators are separated from JavaScript decorator
  checking. Unknown decorators retain ordinary TypeScript diagnostics. This does
  not implement the SDK's complete decorator legality rules or ArkTS lint rules.

This integration uses the conventional ArkUI names above. It does not consume
the SDK fork's configurable `compilerOptions.ets` grammar/annotation tables.
It is not a drop-in replacement for every SDK-specific language-service feature.

## AST and API

Structs reuse `ClassDeclaration`, identified by `NodeFlagsStruct` and
`ast.IsStructDeclaration`. Render calls reuse `CallExpression`, identified by
`NodeFlagsEtsComponent` and `ast.IsEtsComponentExpression`; `EtsBody` holds the
optional content block. Visitors, cloning, printing and the remote AST protocol
preserve this body. The TypeScript API exposes corresponding predicates and
`etsBody`.

Leading-dot receivers use a flagged, zero-width `this` expression. State-style
blocks use a flagged arrow-function representation whose printer preserves block
syntax. These representations retain children and source ranges without text
preprocessing. Kind-specific binding/style-block flags share a bit with the
class-only struct flag, so consumers must check the node kind as well as flags.

The AST wire protocol is version 9; use the matching Go binary and TypeScript API
package. The call-expression factories now take the optional body before flags.
LSP language IDs `ets` and `arkts` select ETS; editors must also associate `.ets`
files with that language ID.

## References and validation

The implementation was informed by:

- Local oxc at `86e108aa76fd2a963bdf702b1cd24084db17f1d0`, particularly
  `crates/oxc_parser/src/js/arkui.rs`, its render-context handling, and
  `crates/oxc_span/src/source_type.rs`.
- [OpenHarmony third_party_typescript](https://github.com/openharmony/third_party_typescript/tree/9cc62fe98f47c0bf113676e3fb33fe932b493052),
  particularly the ETS paths in `src/compiler/parser.ts`, `checker.ts` and
  `types.ts`.

Focused tests live in `parser/arkui_test.go`, `compiler/arkui_test.go`,
`tspath/arkui_test.go` and `api/encoder/arkui_test.go` under `tsc/internal`.
They cover parsing and context isolation, printing and cloning, typed component
calls and styles, imports, declarations, JavaScript emit rejection and the AST
protocol round trip.

```sh
go test ./tsc/internal/parser ./tsc/internal/compiler \
  ./tsc/internal/tspath ./tsc/internal/api/encoder -run TestArkUI
```
