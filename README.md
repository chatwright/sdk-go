# chatwright.dev/sdk

The Go embodiment of the [Chatwright](https://chatwright.dev) run-bundle
standard: the wire model for run-bundle format v1, `Write`/`Read` IO, and the
generated JSON Schema.

A run bundle is the persisted, self-contained artifact a Chatwright run
produces — everything a player (Chatwright Studio), a reviewer or a CI
pipeline needs to see what happened during a run and why it concluded what it
did, with no live emulator, database or network access. This module owns
every type the published schema describes; the runtime that produces bundles
lives in [github.com/chatwright/chatwright](https://github.com/chatwright/chatwright).

## Install

```sh
go get chatwright.dev/sdk
```

## Usage

Read a bundle file (bundles are named `<anything>.chatwright.json`):

```go
package main

import (
	"fmt"
	"os"

	sdk "chatwright.dev/sdk"
)

func main() {
	f, err := os.Open("greetbot-language.chatwright.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	bundle, err := sdk.Read(f)
	if err != nil {
		panic(err)
	}
	for _, run := range bundle.Runs {
		fmt.Printf("run %s on %s (%s): %d part(s)\n",
			run.ID, run.Platform, run.EndpointProfile, len(run.Parts))
	}
}
```

`sdk.Write` is the inverse: deterministic, indented, human-readable JSON,
suitable for checking into a repository and reviewing in a PR diff.

## Schema

The wire shape is published as a JSON Schema (draft 2020-12), generated from
this module's Go types and committed at
[`formats/run-bundle/v1/schema.json`](formats/run-bundle/v1/schema.json):

- Schema `$id`: <https://chatwright.dev/formats/run-bundle/v1/schema.json>
- Format identifier: <https://chatwright.dev/formats/run-bundle/v1>

The Go types are the schema's single source of truth; a drift-guard test
keeps the committed file byte-identical to what the generator produces.

## The standard

Specs, format documentation and design decisions live in the standard
repository, [github.com/chatwright/chatwright](https://github.com/chatwright/chatwright),
and at [chatwright.dev](https://chatwright.dev).

## Licence

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

## Spec-first

Chatwright is developed spec-first with [SpecScore](https://specscore.md/) —
product specs live in the [standard repository](https://github.com/chatwright/chatwright);
this repository's own specs live under [`spec/`](spec/README.md).
