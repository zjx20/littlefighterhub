# Little Fighter Hub - 游戏联机代理

这是一个简单的TCP-over-WebSocket代理工具，旨在帮助不在同一局域网的玩家进行游戏联机。

它的工作原理是：
- 一个**代理服务端（Proxy Server）**运行在有公网IP的服务器上。
- 每个玩家（包括房主）都在本地运行一个**代理客户端（Proxy Client）**。
- **主机端**的代理会建立一个到服务端的“控制连接”来接收指令，并为每个新玩家建立独立的“数据连接”。
- **玩家端**的代理则负责将本地游戏的数据转发到服务端。
- **心跳机制**: 服务端与客户端之间有基于WebSocket Ping/Pong帧的内置心跳，能保持连接穿透NAT，并及时清理断开的连接。

## 编译

确保你的机器上安装了 Go 环境 (版本 >= 1.18)。

```bash
# 编译服务端
go build -o proxy-server ./cmd/proxy-server

# 编译客户端
go build -o proxy-client ./cmd/proxy-client
```

编译后，你会在项目根目录下看到 `proxy-server` 和 `proxy-client` 两个可执行文件。

## 如何运行

### 1. 启动代理服务端

在你的公网服务器上运行：
```bash
./proxy-server
```
服务会启动并监听 `28080` 端口，并为 `host` 和 `peer` 分别提供 `/ws-host` 和 `/ws-peer` 两个连接端点。

### 2. 房主（Host）启动游戏和代理

作为游戏房主，你需要：
1.  先启动你的游戏，并使其作为服务端在本地监听（例如 `127.0.0.1:8080`）。
2.  然后，在你自己的电脑上运行代理客户端，并指定游戏服务的地址：
    ```bash
    # --mode=host 指定为“主机”模式
    # --server=wss://your-domain.com 或 --server=your-ip:28080
    # --game=127.0.0.1:8080 指定你的游戏服务正在监听的地址和端口
    ./proxy-client --mode=host --server=wss://your-domain.com --game=127.0.0.1:8080
    ```

### 3. 其他玩家（Peer）启动代理和游戏

其他加入游戏的玩家，需要：
1.  先启动代理客户端，它会在本地监听一个端口（例如 `127.0.0.1:8081`）：
    ```bash
    # --server=wss://your-domain.com 或 --server=your-ip:28080
    # --local=127.0.0.1:8081 客户端会在本地监听这个地址和端口
    ./proxy-client --mode=peer --server=wss://your-domain.com --local=127.0.0.1:8081
    ```
2.  然后，启动游戏，在游戏里输入服务器地址时，输入 `--local` 参数指定的地址，即 `127.0.0.1:8081`。

## 使用示例（使用 netcat 测试）

这个例子需要4个终端窗口来模拟真实场景。

1.  **终端 A: 启动代理服务端**
    ```bash
    ./proxy-server
    ```

2.  **终端 B: 模拟游戏服务端** (你的游戏)
    ```bash
    # 在 8080 端口监听，等待主机代理来连接
    nc -l 8080
    ```

3.  **终端 C: 启动主机代理**
    ```bash
    # 连接到代理服务，并告知游戏服务在 8080 端口
    ./proxy-client --mode=host --game=localhost:8080 --server=localhost:28080
    ```

4.  **终端 D: 启动玩家代理**
    ```bash
    # 在 8081 端口监听，等待玩家的游戏来连接
    ./proxy-client --mode=peer --local=localhost:8081 --server=localhost:28080
    ```

5.  **终端 E: 模拟玩家的游戏**
    ```bash
    # 连接到玩家代理
    nc localhost 8081
    ```

现在，在**终端 E** 和**终端 B** 之间可以进行实时的双向通信。

## 命令行参数

### `proxy-client`

- `--mode`: 客户端模式。
  - `host`: 主机模式，给游戏房主使用。
  - `peer`: 玩家模式（默认值）。
- `--server`: 代理服务端的地址。支持多种格式，如 `your-domain.com`, `your-ip:28080`, `ws://your-ip:28080`, `wss://your-domain.com`。默认为 `localhost:28080`。
- `--game`: 【仅主机模式】你的游戏服务端监听的地址。默认为 `localhost:8080`。
- `--local`: 【仅玩家模式】为你的游戏客户端提供的本地监听地址。默认为 `localhost:8081`。
