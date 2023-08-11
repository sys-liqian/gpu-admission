# GPU admission

Fork from https://github.com/tkestack/gpu-admission

Refactoring GPU admission using the scheduler framework

Support kubernetes 1.23.x and later versions

Tested on the following kubernetes versions

* kubernetes v1.23.17
* kubernetes v1.25.12


## 1. build docker image

```
$ make img
```

## 2. deploy gpu admission to kubernetes

```
$ make deploy
```
## 3. Test 
```
$ kubectl apply -f test/notebook.yaml
```