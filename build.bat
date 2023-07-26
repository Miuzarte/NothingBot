go mod tidy
go env -w GOOS=linux
go build -ldflags "-s -w" -trimpath -o executable/NothingBot_linux_amd64
go env -w GOOS=windows
go build -ldflags "-s -w" -trimpath -o executable/NothingBot_windows_amd64.exe