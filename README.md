# Yaml language server

## Features

- No configuration needed to detect schemas
- Hover: Show description of field
- Code Action: Open documentation in browser
- Code Action: Fill the field under the cursor with zero values
- Diagnostics: Validate yaml syntax
- Diagnostics: Validate against schema

## Automatically detected schemas

- [Kubernetes Resources](https://github.com/yannh/kubernetes-json-schema).
  Detected from `kind` and `apiVersion`.
- [Custom Resource Definitions](https://github.com/datreeio/CRDs-catalog).
  Detected from `kind` and `apiVersion`.

## Installation

```bash
go install github.com/slarwise/yamlls@latest
```

This will install `yamlls` into `$GOPATH/bin` or `~/go/bin`. Make sure that dir
is in your `$PATH`.

### VS Code

TODO. Do you have to write an extension? Can't you just point to a binary?

### Helix

In your `languages.toml`, add

```toml
[[language]]
name = "yaml"
language-servers = ["yamlls"]

[language-server.yamlls]
command = "yamlls"
# Optional configuration, if you want to override what json schema store returns for a specific
# filename, define the filename and schema url here. Only works with basenames, i.e. it doesn't
# work for schemas where the file pattern is something like '**/.github/workflows/*.yaml'.
config = { filenameOverrides = { '.prettierrc' = "https://my.schema.for.prettier/schema.json" } }
```

### NeoVim

In your `init.lua`, or something sourced from there, add

```lua
vim.api.nvim_create_autocmd('Filetype', {
    pattern = "yaml",
    callback = function()
        vim.lsp.start({
            name = "yamlls",
            cmd = { "yamlls" },
            root_dir = vim.fs.dirname(vim.fs.find(".git", { upward = true, path = vim.api.nvim_buf_get_name(0) })[1]),
        })
    end
})
```

## Development

To try out changes using helix, you can do:

```sh
# Use yamlls in the current directory
export PATH=.:"$PATH"
# Update source code
go build .
hx examples/service.yaml -vvv
# Test things
# Run :log-open inside helix to see helix logs
tail -f ~/Library/Caches/yamlls/log | jq # Follow the logs from yamlls (on a mac)
```

## Potential TODO's

- Completion
  - If `kind` or `apiVersion` is given, give completion suggestions for the
    other
  - Suggest `enum`s from the schema
- Code actions
  - Add all files in the current directory to .resources in a Kustomization file
- Workspace: If there is an kustomization file, connect the resources somehow?
  And give info if there are things that don't match
- Kustomization: Warn when not all files are included in resources
- Better navigation through documentation. Maybe a file explorer view as an alternative to a flat view?
- Support schemas from https://www.schemastore.org
- Support reading the schema from the start of a document, useful when you cannot determine the schema from the contents or filename. E.g. helm values.
- Show the available enums in the documentation
- Make a code action that mimics all available `kubectl <resource> --dry-run=client --output=yaml` with the most basic set of flags needed. So that you can insert a Deployment or an Ingress for example, similar to `fill`. Remove read-only fields like `.status` and `.metadata.creationTimestamp`.

## Bugs

- Can't get description of anyOf, such as github workflow jobs.something.runs-on

## Credits

- The first version of this repo was basically copied from [a-h/examplelsp](https://github.com/a-h/examplelsp), which is an awesome starting point for understanding how to write a language server!
- Uses [json-schema-for-humans](https://github.com/coveooss/json-schema-for-humans) to generate documentation, thank you!
