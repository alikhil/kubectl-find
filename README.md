# kubectl-find

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/alikhil/kubectl-find)
[![Go Report Card](https://goreportcard.com/badge/github.com/alikhil/kubectl-find)](https://goreportcard.com/report/github.com/alikhil/kubectl-find)
![GitHub License](https://img.shields.io/github/license/alikhil/kubectl-find)

It's a plugin for `kubectl` that gives you a **UNIX find**-like experience.

Find resource based on

- **name regex**
- **node name**
- **age**
- **labels**
- **status**

and then **print, patch or delete** any.

## Usage

```
kubectl find [resource type | pods] [flags]

Flags:
  -r, --name string                    Regular expression to match resource names against; if not specified, all resources of the specified type will be returned.
  -n, --namespace string               If present, the namespace scope for this CLI request
  -A, --all-namespaces                 Search in all namespaces; if not specified, only the current namespace will be searched.
      --status string                  Filter pods by their status (phase); e.g. 'Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'.
  -l, --selector string                Label selector to filter resources by labels.
      --max-age string                 Filter resources by maximum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.
      --min-age string                 Filter resources by minimum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.
      --node string                    Filter pods by node name regex; Uses pod.Spec.NodeName or pod.Status.NominatedNodeName if the former is empty.
  -h, --help                           help for kubectl find
  -p, --patch string                   Patch all found resources with the specified JSON patch.
  -e, --exec string                    Execute a command on all found pods.
      --delete                         Delete all matched resources.
  -f, --force                          Skip confirmation prompt before performing actions on resources.
```

## Install

### Using [krew](https://krew.sigs.k8s.io/)

```shell
krew install --manifest-url https://raw.githubusercontent.com/alikhil/kubectl-find/refs/heads/main/krew.yaml
```

### Download binary

Download [latest release](https://github.com/alikhil/kubectl-find/releases) for your platform/os and save it under `$PATH` as `kubectl-find`

## Examples

### Filter using regex

Instead of

```shell
kubectl get pods -o name | grep test
```

Run

```shell
kubectl find -r test
```

### Filter by resource age

```shell
kubectl find cm --min-age 1d -A --name spark
```

### Execute command on several pods

```shell
kubectl find pods -l app=nginx --exec 'nginx -s reload'
```

### Find all failed pods and delete them

```shell
kubectl find pods --status failed -A --delete
```

## Completion

Copy [kubectl_complete-find](https://github.com/alikhil/kubectl-find/blob/main/kubectl_complete-find) script somewhere under `PATH`.
