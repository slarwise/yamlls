# Yaml language server

## Features

- Hover
- Code Action: Open documentation in browser
- Diagnostics: Validate yaml syntax
- Diagnostics: Validate against schema
- No configuration needed for kubernetes, CRD's and schemas that exist in
  json.schemastore.org

## Schema stores

- [Kubernetes](github.com/yannh/kubernetes-json-schema). Detected from `kind`
  and `apiVersion`.
- [Custom Resource Definitions](github.com/datreeio/CRDs-catalog). Detected from
  `kind` and `apiVersion`.
- [Others](json.schemastore.org). Detected from filename.

## Potential TODO's

- [ ] Cache schemas
  - [ ] Cache kubernetes schemas
  - [ ] Cache CRD schemas
  - [ ] Cache schemastore schemas
- [ ] Cache finding schemas
  - [ ] Matching kind and apiVersion against a list of schemas
  - [ ] Matching a filename against a list of schemas
- [ ] Completion
  - [ ] If `kind` or `apiVersion` is given, give completion suggestions for the
        other
  - [ ] Suggest `enum`s from the schema
- [ ] Code actions
  - [ ] Fill all required fields. Use placeholders that fit the type, or the
        first `enum`.
  - [ ] Add all files in the current directory to .resources in a Kustomization
        file
- [ ] Background http server
  - [ ] Show external documentation in a nice way for the currently open file.
        Can do this since we get notifications when the user changes file.
- [ ] Diagnostics
  - [ ] Info diagnostic for Kustomization files when not all files in the
        current dir are included
- [ ] Notifications
  - [ ] Notify when a schema is detected/changed for the current file
- [ ] Workspace: If there is an kustomization file, connect the resources
      somehow? And give info if there are things that don't match

## Bugs

- Can't get description of anyOf, such as github workflow jobs.something.runs-on

## Credits

The first version of this repo was basically copied from
[a-h/examplelsp](https://github.com/a-h/examplelsp), which is an awesome
starting point for understanding how to write a language server!
