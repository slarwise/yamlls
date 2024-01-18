# Yaml language server

## Cool features

- Good ways of figuring out what schema to use, filename kinda sucks
- Start a http server in the background that shows cool visuals. Since we get a
  notification when a user switches file, we can refresh the page to match the
  user's context.
- If there is a kustomization.yaml file, treat the files in the same dir as a
  project. Connect files somehow.
- Go to external documentation, either in the local http server or go to a link.
  Can use different json schema visualizers
- Hover definition is really useful, external documentation can be easier to
  read though
- Would be cool with a code action that fills all fields, or all required
  fields. Similar to fill struct with gopls
- Completion: complete apiVersion would be useful, especially for CRDs. Probably
  need to find these from the file names in the different kubernetes repos.

## TODO

- [ ] JSON schemas:
  - [x] [Kubernetes](github.com/yannh/kubernetes-json-schema). Detect from
        `kind` and `apiVersion`.
  - [ ] [Custom Resource Definitions](github.com/datreeio/CRDs-catalog). Detect
        from `kind` and `apiVersion`.
  - [ ] [Others](json.schemastore.org). Detect from filename.
- [x] Hover
  - [x] Show description of the current key
- [ ] Completion
  - [ ] If `kind` or `apiVersion` is given, give completion suggestions for the
        other
  - [ ] Suggest `enum`s from the schema
- [ ] Code actions
  - [ ] Give link to external documentation
  - [ ] Fill all required fields. Use placeholders that fit the type, or the
        first `enum`.
  - [ ] Add all files in the current directory to .resources in a Kustomization
        file
- [ ] Background http server
  - [ ] Show external documentation in a nice way for the currently open file.
        Can do this since we get notifications when the user changes file.
- [ ] Diagnostics
  - [ ] Validate against the schema
  - [ ] Info diagnostic for Kustomization files when not all files in the
        current dir are included
- [ ] Notifications
  - [ ] Notify when a schema is detected/changed for the current file

## Helix configuration

Create a new language `slarwise` with extension `.slar` to test with.

```toml
[[language]]
name = "slarwise"
language-id = "slar"
scope = "cool.slar"
file-types = ["slar"]
language-servers = ["yamlls"]

[language-server.yamlls]
command = "yamlls"
```

## Credits

This repo is basically copied from
[a-h/examplelsp](https://github.com/a-h/examplelsp), which is an awesome
starting point for understanding how to write a language server!
