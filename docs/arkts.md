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
- The host supplies the SDK's `compilerOptions.ets` tables. Registered component
  names, configured build/builder contexts, Extend/Styles declarations and
  syntax-component callbacks follow OH parser branches. Capitalization does not
  register a component. Explicit empty arrays differ from absent configuration.
- Struct `build()` and `pageTransition()` enable registered component syntax;
  arbitrary render-method names do not. Ordinary callbacks remain isolated.
- Styles/Extend receivers, virtual type parameters/arguments and return types
  come from the configuration and resolve through real SDK symbols. There is no
  synthetic `any` receiver or invented fluent return type. State-style bodies
  retain OH's object-literal shape. Extend takes precedence over Styles even
  when its component argument does not match the table.
- Custom struct calls use the optional property-bag and `LocalStorage` parameters
  from OH `parseStructMembers`; a struct without fields only has the storage
  parameter. `@Require` is enforced by the build transform, not by inventing a
  mandatory TypeScript argument. The parser inserts the virtual constructor and
  configured base class; explicit inheritance wins. Constructor bag properties
  copy explicit types, not initializer-inferred types. Strict initialization
  remains ordinary TS checking. Configured property decorators affect the
  separate use-before-declaration check according to OH's exact conditions.
- Struct `@Param` members without `@Once`, and `@Env(...)`/`@CustomEnv(...)`
  members, receive the source-defined implicit `readonly` modifier.
- Dollar-prefixed names use normal TS symbol lookup. The removed prefix-alias
  inference was not defined by OH. ets2bundle's source preprocessing still owns
  UI-property name collection; the compiler implements the OH semantic-diagnostic
  filtering contract, while the build host supplies the collected names.
- SDK decorators undergo normal TS semantic checking. Source-defined configured
  decorators and Sendable declarations have only the OH grammar exemptions;
  there is no fixed whitelist that bypasses decorator signature checks.
- `ets.libs` paths load as default libraries in the explicit `lib` branch.
  Global symbols declared exclusively in these files or their direct
  triple-slash references are hidden from ordinary TS files. Merged non-ETS
  declarations remain visible, matching `isValidFromLibs`.

Configuration is immutable, content-interned and part of native/JS parse-cache
identity. Use the actual SDK tables; the example above only demonstrates base
project options and does not supply component declarations or ETS tables.

## AST and API

### Annotation declarations and checking

`@interface` is parsed directly in `.ets` and `.d.ets`, including exported and
ambient declarations. No SDK text preprocessing is used. Annotation declarations
and properties carry `NodeFlagsAnnotation`; symbols carry both `Class` and
`Annotation`, have a typed call signature, and have no construct signatures.
The public API preserves these flags and exposes `isAnnotationDeclaration`.
Declaration output preserves the annotation spelling, property defaults,
annotation modifiers and the imports referenced by their expressions. Interface
member annotations are parsed and printed in ETS without changing TS grammar.

The checker validates annotation property types and constant expressions,
required/default arguments, argument types, basic application targets,
duplicates, type/value misuse and import/export restrictions. Diagnostics use
the OH source codes. `Retention` is identified by its declaration name and SDK
source basename, not by guessing the name of the import in application code.

The compiler host must explicitly pass `etsAnnotationsEnable: true` to enable
annotation declarations, as in OH `inAllowAnnotationContext`. Arkdown's build
entry enables it, matching ets2bundle. The option participates in native and
JS API parse-cache identity; it does not enable annotation syntax in `.ts`.

Source-retention policies on SDK `Retention` declarations, the expanded source
annotation target set and use-before-declaration diagnostics are checked. The
API-only `isCompileJsHar` and `moduleRootPath` host options enforce the OH HAR
restriction; the normalized prefix comparison intentionally follows the source.

The SDK `Available`/`SuppressWarnings` declaration identity also selects source
retention. The native checker implements the source-owned content/version and
parent-scope checks, `apiAvailable` argument validation, SDK JSDoc checks and
their `@Available`/`@SuppressWarnings`/guard suppressors. The build host supplies
the callback closure inputs as immutable compiler options. When the SDK declares
CommonJS `apiCheckPlugin`, `annotationCheckPlugin`, or class-style
`apiCheckPlugins`, `--runExternalCode` enables a session-owned Node worker that
preserves the source `require()` ABI, callback order, class construction timing,
SDK TypeScript AST nodes, regular expressions, and module/instance cache. Node
hosts only those SDK JavaScript functions; parsing, binding, type inference,
diagnostics, callback selection, and fallback rules remain in Go. Runtime
annotation metadata lowering remains a Rust/OXC consumer responsibility; it is
not a TypeScript emit feature in the Arkdown integration.

The native checker now implements OH's `@throws` call checks (warning 28040),
including function/method boundaries, try/catch and chained catch handling,
callback exemptions, dependency exclusions, SDK 401 tags, and the five source
`[since]` formats. `compileSdkVersion` and `etsLoaderPath` are build-host options;
an absent version does not impose a version limit. SDK matching preserves the
source's unescaped ECMAScript regular expression, using regexp2's ECMAScript mode.

The `getAnnotationInfo` API accepts an annotation declaration or decorator node
handle and returns declaration-ordered properties, registered type identities,
retention identity, evaluated initializer/argument values, array depth and enum
declaration/first-value facts. Tagged scalar strings preserve negative zero and
non-finite numbers across JSON. These are checker facts, not emitted expressions;
Arkdown's Rust/OXC consumer remains responsible for runtime lowering. Invalid
annotation property types/constants return no info; diagnostics remain available
through the existing diagnostic API.

`getAnnotationTransformInfos` batches those facts for selected files and also
returns the checker-owned import disposition and direct class/method runtime
retention decision used by OH `ohApi.ts::transformAnnotation`.
`getArkTSTransformTypeFacts` batches the resolved property types, `.builder`
receiver identities, and direct identifier-member types consumed by
`process_component_member.ts` and `process_component_build.ts`. It also returns
the SDK declaration module/function identities observed at
`api_check_utils.ts::checkCrossplatformValue`, allowing a native build host to
apply `depsModuleConfig` without reconstructing overload, alias, fluent-return,
or JSDoc resolution outside TypeScript. Its type-kind fields are semantic
booleans rather than the compiler's internal `TypeFlags` numbers: the current
TSGO and OH TypeScript layouts are different, so exposing raw bits would make a
native consumer silently apply the wrong ArkTS rule.
For `@ObjectLink`, the property identity distinguishes unions from intersections
and includes nullable/basic,
`@ObservedV2`, `Function`, and union-constituent facts so the Rust transform can
apply `checkObjectLinkType` without recreating TypeScript inference.

OH mode also preserves 4.9 enum flow semantics: a computed numeric member makes
the enum a regular numeric enum unless a string literal member selected the
literal enum path first. Ordinary TypeScript programs retain the current TSGO
member-literal union behavior.

OH `checker.ts::resolveExternalModule/allowImportSendable` import checks are
implemented for `.so` (warning 28014) and TS-to-ETS imports (28016/28017).
The API host supplies `tsImportSoCheck`, `needDoArkTsLinter`,
`isCompatibleVersion`, and `tsImportSendableEnable`. The `.so` substring branch
precedes ambient lookup, except for strict ETS linter mode; an unresolved `.so`
still warns when checking is disabled. Static Sendable imports/exports and SDK
declarations follow the exact source exceptions, including SDK string-prefix
matching. Dynamic imports and import types are not exempt. The 26 focused cases
check diagnostic categories as well as codes and reject unrelated diagnostics.
OHPM `oh_modules`/`oh-package.json5` resolution, the ets2bundle SDK/kit fallback
order, external-API hiding, `.js` to `.d.ets` substitution, `oh-exports` marking,
and the source-defined TS/ETS import checks are implemented in the native
resolver/checker. `getProgramSourceGraph` exposes the compiler-owned resolved
TS/ETS graph and type-reference files so a build consumer does not recreate
module resolution or the linter's JavaScript-leaf graph rule.

The ArkTS 1.1 linter is an independent diagnostics phase backed by a separate
strict TypeChecker, as in `ArkTSLinter_1_1/LinterRunner.ts`. It implements the
ETS and TS-interoperability rules, `strictCheckerOnly`, OH-module/path exclusions,
Sendable switches, mix-compile behavior, and diagnostic filtering. The complete
OpenHarmony `tests/arkTSTest/testcase` expectation corpus is compared by exact
message and source position in `TestArkTSLinterOpenHarmonyCorpus`.

`disableStrictCheckPaths` follows
`ArkTSLinter_1_1/Utils.ts::configureStrictCheckOHModule` literally: an absent
array selects `node_modules`, `oh_modules`, `build`, and `.preview`; an explicit
empty array selects none; empty and duplicate entries are removed; and
`enableStrictCheckOHModule` removes only the exact `oh_modules` component. Path
components remain case-sensitive. This policy is shared by library-symbol
classification and the strict-diagnostic fallback. The declaration and
`oh_modules` diagnostic filter is activated only by `needDoArkTsLinter`, as in
`program.ts`; enabling another OH option does not silently hide diagnostics.

The API-only `maxFlowDepth` option follows the OH checker's numeric depth
comparison and defaults to 2000 for an absent/zero value. The build host owns
the upstream 2000–65535 range validation; the compiler does not round values.

DSL activation distinguishes bare Builder/LocalBuilder/Styles decorators from
calls and qualified names. Extend/AnimatableExtend require a call with an
identifier component argument; `@Builder()` and bare `@Extend` do not switch
an ordinary call followed by a block into a component expression.

## Source parity boundary

The ArkTS 1.1 type frontend is audited against OpenHarmony
`third_party_typescript` commit
`9cc62fe98f47c0bf113676e3fb33fe932b493052`. For the Arkdown integration the
source-owned type surface consists of parsing and binding ETS nodes, type and
flow inference, semantic and linter diagnostics, OH/kit resolution, SDK API
validation callbacks, and the checker facts consumed by native transforms.
Those paths are implemented in Go and exercised by focused tests plus the full
ArkTS 1.1 linter corpus.

The SDK callback bridge preserves the source distinctions between callback
families: value and format plugin load failures try the next registration;
distribution callbacks retain the last successful value and stop on a later
load or invocation failure; build-version-regex failures stop lookup; and
class plugin load/constructor failures remove that registration. The class
checker receives the complete caller-provided `projectConfig` object, including
fields unknown to the compiler API, while its syscap collections are restored
as JavaScript `Set` objects. The worker loads the OH TypeScript runtime from
`etsLoaderPath/node_modules/typescript`, matching the SDK layout installed by
`developtools_ace_ets2bundle/install_arkguard_tsc_declgen.py`.

API build diagnostics use the compiler's dependency-partitioned normal checker
pool and retain that pool for the immutable Program lifetime. Batched ArkTS
transform facts run on the same file/checker associations, so they reuse the
types populated by diagnostics instead of constructing a second service
checker. The strict ArkTS linter keeps its independent checker pool, matching
the source's separate linter program. Parallel fact responses are written by
their original request index; concurrency therefore does not change the public
ordering.

The following source changes are deliberately not separate TSGO capabilities:

- JavaScript lowering, annotation metadata emission, ArkUI runtime transforms,
  obfuscation and source-map rewriting are production Rust/OXC responsibilities.
- `convertTsAstToJsAst` has no production caller in the current OH compiler or
  ets2bundle source; no unused ESTree compatibility API is reproduced.
- builder/document-registry cache flags and checker release hooks are lifecycle
  optimizations. TSGO uses content-keyed snapshots, parse caches and explicit
  checker ownership instead of copying the JavaScript cache architecture.
- Standard TypeScript behavior added by the newer TSGO base is not an ArkTS
  extension gap. Product configuration choosing a language version is likewise
  a build-host policy, not a missing type-checker capability.

`node tools/scripts/package-arkts.mjs` builds the three existing release targets
without replacing an older directory. A dirty build has an explicit worktree
fingerprint; `BUILD.json` and `SHA256SUMS` record binary/archive identities.
The source diff and added files are retained alongside the packages. Building a
foreign target does not constitute runtime validation on that platform.

### Shared syntax storage

Structs reuse `ClassDeclaration`, identified by `NodeFlagsStruct` and
`ast.IsStructDeclaration`. Render calls reuse `CallExpression`, identified by
`NodeFlagsEtsComponent` and `ast.IsEtsComponentExpression`; `EtsBody` holds the
optional content block. Visitors, cloning, printing and the remote AST protocol
preserve this body. The TypeScript API exposes corresponding predicates and
`etsBody`.

Virtual constructor/receiver/type nodes carry a separate `virtual` marker,
preserved in Go/JS cloning and encoding. Virtual nodes are not missing nodes;
token positions and trailing-comma detection respect this distinction. Source
printing and reparsing is not a substitute for transporting the virtual AST.

The AST wire protocol is version 10 (48-byte header, 32-byte nodes); use the matching Go binary and TypeScript API
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

Focused tests live under `tsc/internal` in the parser, compiler, resolver,
project and API packages. They cover parsing and context isolation, printing
and cloning, typed component calls and styles, annotation/retention facts,
SDK callbacks, OH resolution, import rules, diagnostics, source-graph queries,
the independent linter, JavaScript emit rejection and AST protocol round trips.

```sh
go test ./tsc/internal/parser ./tsc/internal/compiler \
  ./tsc/internal/tspath ./tsc/internal/api/encoder -run TestArkUI

OH_TYPESCRIPT_SOURCE=/path/to/third_party_typescript \
  go test ./tsc/internal/compiler -run TestArkTSLinterOpenHarmonyCorpus -count=1
```
