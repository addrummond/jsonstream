Testing on linux/amd64 and linux/i386. TODO: Incorporate linux/i386 into
GitHub actions.

```sh
docker run --rm -it --platform linux/i386 -v "$PWD":/usr/src/myapp -w /usr/src/myapp golang:1.23 go test
docker run --rm -it --platform linux/amd64 -v "$PWD":/usr/src/myapp -w /usr/src/myapp golang:1.23 go test
```
