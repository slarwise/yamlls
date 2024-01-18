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

## File detection

- Kubernetes: Everything kubernetes has `kind` and `apiVersion` on the top
  level. If the user has entered one of them, we can give completions for the
  other.
- github actions? Can look at the filepath, if it is under github/workflows then
  we can be sure.
- Other stuff? Not sure yet.
- Should we treat kubernetes and github actions as their own languages? That
  would simplify the language server, if we have separate ones for each. For
  example, bash files use the shebang to identify the language, not the file
  extension.

## Json schemas

- Kubernetes: yannh/kubernetes-json-schema
- Kubernetes Custom Resource Definitions: datreeio/CRDs-catalog
- Github and others: json.schemastore.org
