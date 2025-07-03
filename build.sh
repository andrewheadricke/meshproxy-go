export CGO_ENABLED=0
export GOARCH=arm
export GOARM=6
go build -o meshproxy-go-arm
upx meshproxy-go-arm
