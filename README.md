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
