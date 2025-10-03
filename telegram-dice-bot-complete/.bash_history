ip addr show
netstat -tuln
ss -tuln
go version
snap install go --classic
go version
mkdir -p telegram-dice-bot
cd telegram-dice-bot && go mod init telegram-dice-bot
mkdir -p internal/{bot,config,database,game,models,utils}
go mod tidy
cd /root/telegram-dice-bot && go test ./test/... -v
cd /root/telegram-dice-bot && CGO_ENABLED=1 go test ./test/... -v
apt update && apt install -y gcc
cd /root/telegram-dice-bot && CGO_ENABLED=1 go test ./test/... -v
cd /root/telegram-dice-bot && CGO_ENABLED=1 go build -o bin/telegram-dice-bot main.go
cd /root/telegram-dice-bot && CGO_ENABLED=1 go test ./test/... -v
cd /root/telegram-dice-bot && CGO_ENABLED=1 go build -o bin/telegram-dice-bot main.go
cd /root/telegram-dice-bot && mkdir -p data
cd /root/telegram-dice-bot && CGO_ENABLED=1 ./bin/telegram-dice-bot
cd /root/telegram-dice-bot && go get github.com/joho/godotenv
cd /root/telegram-dice-bot && CGO_ENABLED=1 go build -o bin/telegram-dice-bot main.go
cd /root/telegram-dice-bot && ./bin/telegram-dice-bot
./bin/telegram-dice-bot
cd /root/telegram-dice-bot && go build -o bin/telegram-dice-bot cmd/main.go
cd /root/telegram-dice-bot && go build -o bin/telegram-dice-bot
go build -o bin/telegram-dice-bot
cd ~/Downloads  # 切换到下载文件夹
scp root@192.3.249.23:/root/telegram-dice-bot/telegram-dice-bot-ULTRA-COMPLETE-WITH-CACHE-20251003-053635.tar.gz ./
