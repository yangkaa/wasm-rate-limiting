git pull origin main
tinygo build -o main.wasm -scheduler=none -target=wasi main.go
docker build . -t yangk/wasm:${VERSION:-v7}
docker push yangk/wasm:${VERSION:-v7}
