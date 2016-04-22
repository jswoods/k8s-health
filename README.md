# How to build

Building is simple: either run ./build manually or let jenkins do it.

# If you have to update the Godeps

If dependencies are added or need to be updated, you should manually run the ./build_godeps script.
Note that you will need "godep" installed, either via brew or "go get". Note that if you use "go get",
you'll have to put this in a go-tools directory and make sure that comes after your gopath. E.g.

```
export GO_TOOLS=~/somedir/go-tools
export GOPATH=~/somedir:$GO_TOOLS
```

Make sure that if you want to use the dependencies set up via build_godeps, you will need to
set your GOPATH correctly. E.g.

```
export GOPATH=`pwd`/tmp/go
```

Once the Godeps are updated, you should commit the new Godeps/Godeps.json file.

# More docs

[godep](https://github.com/tools/godep)
[godep on kubernetes](https://github.com/kubernetes/kubernetes/blob/master/docs/devel/development.md)
[kubernetes client api](https://godoc.org/k8s.io/kubernetes/pkg/client)
