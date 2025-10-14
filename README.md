# kubectl find

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/alikhil/kubectl-find)
[![Go Report Card](https://goreportcard.com/badge/github.com/alikhil/kubectl-find)](https://goreportcard.com/report/github.com/alikhil/kubectl-find)
![GitHub License](https://img.shields.io/github/license/alikhil/kubectl-find)

It's a plugin for `kubectl` that gives you a **UNIX find**-like experience.

Find resource based on

- **name regex**
- **age**
- **labels**
- **status**
- **node name** (for pods only)
- **restarts** (for pods only)
- **image name** (for pods only)
- **jq filter** - custom condition

and then **print, patch or delete** any.

## Usage

```shell
kubectl fd [resource type | pods] [flags]

Flags:
  -r, --name string                    Regular expression to match resource names against; if not specified, all resources of the specified type will be returned.
  -n, --namespace string               If present, the namespace scope for this CLI request
  -A, --all-namespaces                 Search in all namespaces; if not specified, only the current namespace will be searched.
      --status string                  Filter pods by their status (phase); e.g. 'Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'.
      --image string                   Regular expression to match container images against.
  -j, --jq string                      jq expression to filter resources; Uses gojq library for evaluation.
      --restarted                      Find pods that have been restarted at least once.
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
krew install fd
```

### Download binary

Download [latest release](https://github.com/alikhil/kubectl-find/releases) for your platform/os and save it under `$PATH` as `kubectl-fd`

## Examples

### Filter by jq

Based on [gojq](https://github.com/itchyny/gojq) implementation of `jq`.
Check resource structure on [kubespec.dev](https://kubespec.dev/).

#### Find pods with empty nodeSelector

```shell
kubectl fd pods -j '.spec.nodeSelector == null' -A
```

#### Find pods with undefined resources

```shell
kubectl fd pods -j 'any( .spec.containers[]; .resources == {} )' -A
```

### Filter using regex

Instead of

```shell
kubectl get pods -o name | grep test
```

Run

```shell
kubectl fd -r test
```

### Filter by resource age

```shell
kubectl fd cm --min-age 1d -A --name spark
```

### Execute command on several pods

```shell
kubectl fd pods -l app=nginx --exec 'nginx -s reload'
```

### Find all failed pods and delete them

```shell
kubectl fd pods --status failed -A --delete
```

### Find restarted pods

```shell
kubectl fd --restarted
```

## Completion

Copy [kubectl_complete-fd](https://github.com/alikhil/kubectl-find/blob/main/kubectl_complete-fd) script somewhere under `PATH`.

## Star History

<a href="https://www.star-history.com/#alikhil/kubectl-find&type=date&legend=bottom-right">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=alikhil/kubectl-find&type=date&theme=dark&legend=bottom-right" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=alikhil/kubectl-find&type=date&legend=bottom-right" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=alikhil/kubectl-find&type=date&legend=bottom-right" />
 </picture>
</a>
