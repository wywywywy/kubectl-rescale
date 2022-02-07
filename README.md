# kubectl-rescale

A `kubectl` plugin to scale a deployment or statefulset to 0 then back up to the original desired replica count.

## Install

### Manually

Download from the [releases page](https://github.com/wywywywy/kubectl-rescale/releases) and put the binary into your path.

### With Krew

To install using [krew](https://krew.sigs.k8s.io/) the kubectl plugin manager, simply do:

```
kubectl krew install rescale
```

## Usage

```
Usage:
  kubectl rescale [name of deployment/statefulset] [flags]

Examples:

        # scale a deployment to 0 replicas then back up to the original count
        kubectl rescale deployment/nginx

        # scale a statefulset to 0 replicas then back up to the original count
        kubectl rescale statefulset/mysql

        # scale a statefulset to 0 replicas then back up to the original count, and wait for a maximum of 600 seconds to do so
        kubectl rescale statefulset/mysql --max-wait-seconds=600

        # it also supports short names
        kubectl rescale sts/mysql

        # if the kind is not provided, it will first try to find a deployment with the supplied name, and if not found then statefulset
        kubectl rescale nginx

        # a namespace can also be supplied
        kubectl rescale deployment/nginx -n dev
```


## Contributing

Yes, contributions are always welcome.  
Fork it & submit a pull request.

## License

This is licensed under the Apache License 2.0.