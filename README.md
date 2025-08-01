# Little Fighter Hub - 游戏联机代理

这是一个简单的TCP-over-WebSocket代理工具，旨在帮助不在同一局域网的玩家进行游戏联机。

它的工作原理是：
- 一个**代理服务端（Proxy Server）**运行在有公网IP的服务器上。
- 每个玩家（包括房主）都在本地运行一个**代理客户端（Proxy Client）**。
- **主机端**的代理会建立一个到服务端的“控制连接”来接收指令，并为每个新玩家建立独立的“数据连接”。
- **玩家端**的代理则负责将本地游戏的数据转发到服务端。
- **应用层心跳**: 服务端与客户端之间的所有连接都使用基于JSON文本消息的自定义心跳（`{"type":"ping"}`/`{"type":"pong"}`），以确保在复杂的网络环境下也能保持连接活性，并及时清理断开的连接。

## 编译

确保你的机器上安装了 Go 环境 (版本 >= 1.18)。

```bash
# 编译服务端
# GOPROXY=goproxy.cn 使用 Go 代理加速编译
GOPROXY=goproxy.cn go build -o proxy-server ./cmd/proxy-server

# 编译客户端
# GOPROXY=goproxy.cn 使用 Go 代理加速编译
GOPROXY=goproxy.cn go build -o proxy-client ./cmd/proxy-client
```

编译后，你会在项目根目录下看到 `proxy-server` 和 `proxy-client` 两个可执行文件。

### 交叉编译 (Cross-compilation)

Go 语言支持方便的交叉编译。例如，如果你想在 macOS 或 Linux 上为 Windows 编译客户端：

```bash
# GOOS=windows 指定目标系统为 Windows
# GOARCH=amd64 指定目标架构为 64-bit
# GOPROXY=goproxy.cn 可以使用 Go 代理加速编译
# -o proxy-client.exe 指定输出文件名为 proxy-client.exe
GOOS=windows GOARCH=amd64 GOPROXY=goproxy.cn go build -o proxy-client.exe ./cmd/proxy-client
```

如果你正在使用 Windows 系统，可以直接使用 `go build` 命令，它会默认生成 `.exe` 文件。

> **注意**: 在 Windows 的 Command Prompt (CMD) 中，设置环境变量的语法不同，你需要分步执行：
> ```cmd
> set GOOS=windows
> set GOARCH=amd64
> set GOPROXY=goproxy.cn
> go build -o proxy-client.exe ./cmd/proxy-client
> ```

## 游戏联机实战

1.  **部署服务端**
    将编译好的 `proxy-server` 文件部署到有公网 IP 的服务器上，并运行它。假设服务器的可访问地址是 `your-server.com`。
    ```bash
    # 服务端将默认在 8095 端口上运行
    ./proxy-server
    ```

2.  **房主 (Host) 操作**
    -   首先，在游戏内启动游戏自带的 Room Server。这通常会在本地 `127.0.0.1:8080` 监听。
    -   然后，在本地启动 `proxy-client`，并设置为 `host` 模式，指向你的游戏 Room Server。
        ```bash
        # --server 指向你的公网代理服务，注意端口是 8095
        # --game 指向你本地的游戏Room Server地址
        # --room 设置一个房间密码，例如 "12345"
        ./proxy-client --mode=host --server=your-server.com:8095 --game=127.0.0.1:8080 --room=12345
        ```

3.  **其他玩家 (Peer) 操作**
    -   在本地启动 `proxy-client`，并设置为 `peer` 模式。
        ```bash
        # --server 同样指向公网代理服务，注意端口是 8095
        # --local 指定一个本地端口，游戏将通过这个端口连接
        # --room 必须和房主设置的房间密码完全一致
        ./proxy-client --mode=peer --server=your-server.com:8095 --local=127.0.0.1:8080 --room=12345
        ```

4.  **进入游戏**
    -   现在，**所有玩家**（包括房主）都可以在游戏的多人联机界面输入 `localhost` 和 `8080` 端口来加入游戏。
    -   **原理**:
        -   对于房主，游戏直接连接到本地 `127.0.0.1:8080` 上由游戏自己启动的 Room Server。
        -   对于其他玩家，游戏连接到本地 `127.0.0.1:8080` 上由 `proxy-client` 监听的端口，`proxy-client` 会将所有数据通过公网的 `proxy-server` 转发给房主的 `proxy-client`，最终到达房主的游戏 Room Server。

## 通用测试（使用 netcat）

这个例子需要多个终端窗口来模拟真实场景。

1.  **终端 A: 启动代理服务端**
    ```bash
    # 默认端口为 8095
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
    ./proxy-client --mode=host --game=localhost:8080 --server=localhost:8095
    ```

4.  **终端 D: 启动玩家代理**
    ```bash
    # 在 8081 端口监听，等待玩家的游戏来连接
    ./proxy-client --mode=peer --local=localhost:8081 --server=localhost:8095
    ```

5.  **终端 E: 模拟玩家的游戏**
    ```bash
    # 连接到玩家代理
    nc localhost 8081
    ```

现在，在**终端 E** 和**终端 B** 之间可以进行实时的双向通信。

## 使用浏览器测试 WebSocket

项目提供了一个 `tests/test_ws.html` 文件，可以用来直接在浏览器中测试 `proxy-server` 的 `host` 端 WebSocket 连接。

1.  **启动代理服务端**
    在你的服务器或本地启动 `proxy-server`。
    ```bash
    ./proxy-server -port 8095
    ```

2.  **用浏览器打开测试页面**
    直接在你的电脑上用浏览器打开 `tests/test_ws.html` 文件。

3.  **连接到服务**
    -   在页面的输入框中，填入你的 `proxy-server` 的 `host` 端地址。格式为 `ws://<your-server-ip>:8095/ws-host?room=<your-room-id>`。
    -   例如，如果服务运行在本地，你可以填入：`ws://localhost:8095/ws-host?room=test`。
    -   点击 "Connect" 按钮。

4.  **观察结果**
    -   如果连接成功，页面会显示 "Connected"。
    -   浏览器开发者工具的控制台（Console）会打印出 `websocket open` 日志，并开始定期发送 `pong` 消息。
    -   你可以在下方的输入框中发送自定义消息进行测试。

这个测试页面模拟了一个 `host` 客户端的行为，对于快速验证服务端是否正常工作非常有用。

## 命令行参数

### `proxy-server`

- `-port`: 指定代理服务监听的端口。默认为 `8095`。

### `proxy-client`

- `--mode`: 客户端模式。
  - `host`: 主机模式，给游戏房主使用。
  - `peer`: 玩家模式（默认值）。
- `--server`: 代理服务端的地址。支持多种格式，如 `your-domain.com`, `your-ip:8095`, `ws://your-ip:8095`, `wss://your-domain.com`。默认为 `localhost:8095`。
- `--room`: 指定一个房间ID（任意字符串）。只有使用相同房间ID的客户端才会被分配到同一个逻辑房间中进行通信。默认为 `default`。
- `--game`: 【仅主机模式】你的游戏服务端监听的地址。默认为 `localhost:8080`。
- `--local`: 【仅玩家模式】为你的游戏客户端提供的本地监听地址。默认为 `localhost:8081`。
