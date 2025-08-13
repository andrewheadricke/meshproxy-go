export CGO_ENABLED=0
export GOARCH=arm
export GOARM=6
go build -a -ldflags "-s -w" -trimpath -o meshproxy-go-arm -tags withui
upx meshproxy-go-arm
