# LF2 联机网络协议分析

## Room Server WebUI

启动 room server 后，在浏览器访问 http://localhost:8080/ 就能打开同款 webui，方便分析这部分的交互协议。

### YOUR_ID 消息

建立websocket连接之后，服务端主动发送一行 YOUR_ID，紧接着是一个自增ID；后面的 `200 -999 ...` 可能没有特殊意义，从 lf2-room-server-win.exe 里的文本来看，这几个值是写死的。

```
YOUR_ID
5
200
-999
-999
-999
```

### ADMIN 命令

webui 在建立连接后主动发送 ADMIN 命令，表明连接用于 ADMIN 管理，这个命令没有任何参数。之后服务器每隔 5 秒发送一条 STATS 和 ROOM_LIST 消息。

```
ADMIN
```

### ROOM_LIST 消息

```
ROOM_LIST
Room 1 [VACANT] 3 5137
Room 2 [VACANT] 3 5137
Room 3 [VACANT] 3 5137
Room 4 [VACANT] 3 5137
Room 5 [VACANT] 3 5137
Room 6 [VACANT] 3 5137
Room 7 [VACANT] 3 5137
Room 8 [VACANT] 3 5137
```

这是最开始的状态，共有 8 行记录，格式为 `Room <room id> <state> <latency> <time>`。

其中 state 可能取值 `VACANT`、`LOBBY` 和 `STARTED`，time 则是服务端启动至今的毫秒数。

当有一名玩家加入房间后，消息变为：

```
ROOM_LIST
Room 1 [LOBBY] 3 4604 {Name: X, ID: 3, IP: local}
Room 2 [VACANT] 3 30012
Room 3 [VACANT] 3 30012
Room 4 [VACANT] 3 30012
Room 5 [VACANT] 3 30012
Room 6 [VACANT] 3 30012
Room 7 [VACANT] 3 30012
Room 8 [VACANT] 3 30012
```

可以看到房间状态变为 `LOBBY`，末尾增加了玩家的信息。

两名玩家在同一房间的情况：

```
ROOM_LIST
Room 1 [LOBBY] 3 120482 {Name: X, ID: 12, IP: local}, {Name: Y, ID: 13, IP: ::ffff:192.168.123.5}
Room 2 [VACANT] 3 551588
Room 3 [VACANT] 3 74253394
Room 4 [VACANT] 3 74253394
Room 5 [VACANT] 3 74253394
Room 6 [VACANT] 3 74253394
Room 7 [VACANT] 3 74253394
Room 8 [VACANT] 3 74253394
```

已开始游戏的情况：

```
ROOM_LIST
Room 1 [STARTED] 3 71539 {Name: X, ID: 5, IP: local}, {Name: Y, ID: 6, IP: ::ffff:192.168.123.5}
Room 2 [VACANT] 3 8903496
Room 3 [VACANT] 3 8903496
Room 4 [VACANT] 3 8903496
Room 5 [VACANT] 3 8903496
Room 6 [VACANT] 3 8903496
Room 7 [VACANT] 3 8903496
Room 8 [VACANT] 3 8903496
```

### STATS 消息

服务端定期发送统计消息，包含两个数字，第一个数字是“总游玩时间”，第二个是“总玩家数”。暂时不清楚这些数据是如何统计的，不过不太关键。

```
STATS 0 0
```

## 游戏客户端

游戏客户端点击连接到服务器后的交互细节，主要是房间相关的操作。

连接之后服务器同样会发一条 YOUR_ID 消息。

### LIST 命令

```
LIST
```

客户端建立连接后马上会发一条 LIST 命令，列出当前房间的状态。

服务端回复：

```
LIST

¶
Room
1
LOBBY
3
128483
2
X, Y
¶
Room
2
VACANT
3
136390
0

¶
Room
3
VACANT
3
136390
0

¶
Room
4
VACANT
3
136390
0

¶
Room
5
VACANT
3
136390
0

¶
Room
6
VACANT
3
136390
0

¶
Room
7
VACANT
3
136390
0

¶
Room
8
VACANT
3
136390
0

```

格式为 `Room\n<room id>\n<state>\n<latency>\n<time>\n<number of players>\n<name of player1>, <name of player2>...`，每个房间开头是“¶\n”，hex 值为 "c2 b6 0a"。

### JOIN 命令

```
JOIN
1
X
X
P2
P3
P4
STAGE_1_EASY,STAGE_1_NORMAL,STAGE_1_DIFFICULT,STAGE_1_CRAZY,STAGE_2_EASY,STAGE_2_NORMAL,STAGE_2_DIFFICULT,STAGE_2_CRAZY,STAGE_3_EASY,STAGE_3_NORMAL,STAGE_3_DIFFICULT,STAGE_3_CRAZY,STAGE_4_EASY,STAGE_4_NORMAL,STAGE_4_DIFFICULT,STAGE_4_CRAZY,STAGE_5_EASY,STAGE_5_NORMAL,STAGE_5_DIFFICULT,STAGE_5_CRAZY,GOLD_DAVIS,GOLD_WOODY,GOLD_DENNIS,GOLD_FREEZE,GOLD_FIREN,GOLD_LOUIS,GOLD_RUDOLF,GOLD_HENRY,GOLD_JOHN,GOLD_DEEP,GOLD_BAT,GOLD_LOUISEX,GOLD_FIRZEN,GOLD_JULIAN,SILVER_DAVIS,SILVER_WOODY,SILVER_DENNIS,SILVER_FREEZE,SILVER_FIREN,SILVER_LOUIS,SILVER_RUDOLF,SILVER_HENRY,SILVER_JOHN,SILVER_DEEP,SILVER_BAT,SILVER_LOUISEX,SILVER_FIRZEN,SILVER_JULIAN,BRONZE_DAVIS,BRONZE_WOODY,BRONZE_DENNIS,BRONZE_FREEZE,BRONZE_FIREN,BRONZE_LOUIS,BRONZE_RUDOLF,BRONZE_HENRY,BRONZE_JOHN,BRONZE_DEEP,BRONZE_BAT,BRONZE_LOUISEX,BRONZE_FIRZEN,BRONZE_JULIAN,SURVIVAL_10,SURVIVAL_20,SURVIVAL_30,SURVIVAL_40,SURVIVAL_50,SURVIVAL_60,SURVIVAL_70,SURVIVAL_80,SURVIVAL_90,SURVIVAL_100,COOP_2P,COOP_3P,COOP_4P,COOP_6P,COOP_8P,MODE_VS1,MODE_VS2,MODE_BATTLE1,MODE_BATTLE2,MODE_1_ON_1_CHAMP,MODE_2_ON_2_CHAMP
```

格式为 `JOIN\n<room id>\n<player name>\n<p1 name>\n<p2 name>\n<p3 name>\n<p4 name>\n<achievements>`。


服务端广播 PLAYER_LIST 消息：

```
PLAYER_LIST
1
3
¶
5
X
X
P2
P3
P4
STAGE_1_EASY,STAGE_1_NORMAL,STAGE_1_DIFFICULT,STAGE_1_CRAZY,STAGE_2_EASY,STAGE_2_NORMAL,STAGE_2_DIFFICULT,STAGE_2_CRAZY,STAGE_3_EASY,STAGE_3_NORMAL,STAGE_3_DIFFICULT,STAGE_3_CRAZY,STAGE_4_EASY,STAGE_4_NORMAL,STAGE_4_DIFFICULT,STAGE_4_CRAZY,STAGE_5_EASY,STAGE_5_NORMAL,STAGE_5_DIFFICULT,STAGE_5_CRAZY,GOLD_DAVIS,GOLD_WOODY,GOLD_DENNIS,GOLD_FREEZE,GOLD_FIREN,GOLD_LOUIS,GOLD_RUDOLF,GOLD_HENRY,GOLD_JOHN,GOLD_DEEP,GOLD_BAT,GOLD_LOUISEX,GOLD_FIRZEN,GOLD_JULIAN,SILVER_DAVIS,SILVER_WOODY,SILVER_DENNIS,SILVER_FREEZE,SILVER_FIREN,SILVER_LOUIS,SILVER_RUDOLF,SILVER_HENRY,SILVER_JOHN,SILVER_DEEP,SILVER_BAT,SILVER_LOUISEX,SILVER_FIRZEN,SILVER_JULIAN,BRONZE_DAVIS,BRONZE_WOODY,BRONZE_DENNIS,BRONZE_FREEZE,BRONZE_FIREN,BRONZE_LOUIS,BRONZE_RUDOLF,BRONZE_HENRY,BRONZE_JOHN,BRONZE_DEEP,BRONZE_BAT,BRONZE_LOUISEX,BRONZE_FIRZEN,BRONZE_JULIAN,SURVIVAL_10,SURVIVAL_20,SURVIVAL_30,SURVIVAL_40,SURVIVAL_50,SURVIVAL_60,SURVIVAL_70,SURVIVAL_80,SURVIVAL_90,SURVIVAL_100,COOP_2P,COOP_3P,COOP_4P,COOP_6P,COOP_8P,MODE_VS1,MODE_VS2,MODE_BATTLE1,MODE_BATTLE2,MODE_1_ON_1_CHAMP,MODE_2_ON_2_CHAMP
```

PLAYER_LIST 字段大致是 `<room id>\n<latency>\n¶<player 1>\n¶<player 2>\n...`，而 player 的格式为 `<player id>\n<player name>\n<p1 name>\n<p2 name>\n<p3 name>\n<p4 name>\n<achievements>`。

问题：房间超过8人会回复什么？
问题：房间处于STARTED状态会回复什么？
问题：房间号不存在回复什么？

### CHANGE_LATENCY 命令

玩家在 webui 上调整 latency，就会触发 CHANGE_LATENCY 命令，带有一个数字参数，表示新的latency值。

```
CHANGE_LATENCY
4
```

服务端广播 PLAYER_LIST 消息。

### LEAVE 命令

玩家在 webui 上点击 “离开房间”，会触发 LEAVE 命令，参数是 room id。

```
LEAVE
2
```

服务端广播 LEFT_ROOM 消息。

```
LEFT_ROOM
2
```

### START 命令

房间里任意一名玩家点击“开始游戏”，就会触发 START 命令，没有任何参数。

```
START
```

服务端会广播 ROOM_NOW_STARTED 消息（点击“开始”的玩家也会收到）：

```
ROOM_NOW_STARTED
1
395698
```

消息格式应该是 `ROOM_NOW_STARTED\n<room id>\n<time>`。

### CHAT 命令

玩家点击“开始游戏”之后，客户端还会立马发送一个 CHAT 命令，带一个字符串参数：

```
CHAT
clicked 'Start Game'
```

服务端广播 CHAT 消息给所有玩家：

```
CHAT
12
X
clicked 'Start Game'
```

格式为`CHAT\n<player id>\n<player name>\n<message>`。

### FRAME 命令

开始游戏后，客户端开始不断发送 FRAME 包，服务端收到后转发给其他玩家

```
FRAME
2
14037
2
0
0
0
0
34314
```

前两个数字分别是 player id 和 frame seq ，紧接着的4个数字是四个键位当前的按键情况。倒数第二个数字含义不明。最后一个数字是校验值，游戏状态不一致就是通过这个值来判断的。

room server 并不需要理解或处理 FRAME 包，它只需要将数据转发给其他客户端即可。


### AWAY 命令

玩家点击修改控制设定时，会触发 AWAY 命令：

```
AWAY
3
control_setting
```

`3` 是 player id。

服务端会将消息广播给其他玩家（不包括发出命令的玩家）。

修改完成后，会发送另一条 AWAY 命令，里面有 resume 字样。

```
AWAY
3
resume
```

服务端同样会广播这条消息给其他玩家。

### UPDATE_CONTROL_NAMES 命令

玩家修改控制设定时，如果修改了玩家名称，会触发 UPDATE_CONTROL_NAMES 命令：

```
UPDATE_CONTROL_NAMES
3
P1 YY
P2
P3
P4
```

`3` 是 player id，后面依次是4个键位的名字。

服务端把消息发送给其他玩家（不包括发出命令的玩家）。


### UPDATE_ACHIEVEMENTS 命令

TODO 有待抓包

### 联机后有玩家退出游戏

如果有一方玩家退出（比如关闭游戏），退出时会正常关闭 websocket 连接，但不会发送应用层消息。服务端会自动向其他玩家推送 "left the Room" 消息：

```
CHAT
7
Y
left the Room.
```

然后推送 PLAYER_LIST 消息。

## 关于 Latency 设定

房间中有一个约定的 Latency 值，这个值跟玩家双方的端到端网络延迟有关。游戏中并没有详细解释这个值的作用和原理，这里做一个猜测。

在帧同步游戏中，为了处理网络延迟，通常会有一个“输入缓冲”。这个缓冲的目的不是为了延迟发送，而是为了让双方有足够的时间来接收对方的输入。

让我们以一个约定的3帧延迟为例：

1. 玩家A在第10帧按下了一个键。
2. 玩家A的电脑会立即将这个按键输入记录下来，并将其发送给玩家B。
3. 玩家A的电脑并不会立即执行这个输入。它会等待，直到第13帧。
4. 当到达第13帧时，玩家A的电脑会去查看：
  * 自己的第10帧输入是否已经接收到了（当然，它自己有）。
  * 玩家B的第10帧输入是否也已经接收到了。
5. 如果玩家A和玩家B都收到了彼此的第10帧输入，那么双方的游戏引擎会在第13帧同步执行第10帧的输入。

这个过程可以理解为：双方都等待接收到所有玩家的第N帧输入后，才会在第N+3帧执行这些输入。

## 总结

实现一个 Room Server 需要处理两类连接：**游戏客户端**和 **WebUI 管理端**。服务器通过连接建立后收到的第一条消息来区分它们（`ADMIN` 来自 WebUI，其他来自游戏客户端）。

服务器的核心职责是维护房间列表（最多8个）以及每个房间内的玩家状态，并根据收到的命令进行响应或广播。

### 核心处理逻辑

1.  **连接管理**:
    *   **新连接**: 接受 WebSocket 连接后，立即发送 `YOUR_ID` 消息，分配一个唯一的客户端 ID。
    *   **断开连接**: 如果该连接对应一个在房间内的玩家，则向该房间的其他所有玩家广播一条 `CHAT` 消息（内容为 `<Player Name> left the Room.`），并紧接着广播更新后的 `PLAYER_LIST`。

2.  **命令处理**:
    *   `ADMIN`: 将当前连接标记为管理端。此后，定期（如每5秒）向该连接发送 `STATS` 和 `ROOM_LIST` 消息。
    *   `LIST`: 响应发起请求的客户端，发送包含所有房间当前状态的 `LIST` 消息。
    *   `JOIN`:
        *   将玩家添加进指定房间。
        *   向该房间的所有玩家（包括新加入的）广播 `PLAYER_LIST` 消息。
        *   需要处理房间满员、游戏已开始或房间不存在等异常情况（具体响应待定）。
    *   `LEAVE`:
        *   从指定房间移除玩家。
        *   向该房间的所有玩家广播 `LEFT_ROOM` 消息，告知玩家ID。
        *   （可选）可以紧接着广播更新后的 `PLAYER_LIST`。
    *   `START`:
        *   将房间状态标记为 `STARTED`。
        *   向该房间的所有玩家广播 `ROOM_NOW_STARTED` 消息。
    *   `CHAT`:
        *   向该玩家所在房间的所有玩家（包括发送者自己）广播 `CHAT` 消息，消息中包含发送者的 ID、名称和聊天内容。
    *   `FRAME`:
        *   将收到的 `FRAME` 消息原封不动地转发给同一房间的所有**其他**玩家。服务器不解析其内容。
    *   `AWAY`:
        *   将收到的 `AWAY` 消息原封不动地转发给同一房间的所有**其他**玩家。
    *   `UPDATE_CONTROL_NAMES`:
        *   将收到的 `UPDATE_CONTROL_NAMES` 消息原封不动地转发给同一房间的所有**其他**玩家。
    *   `CHANGE_LATENCY`:
        *   更新房间的 `latency` 值。
        *   向该房间的所有玩家广播更新后的 `PLAYER_LIST` 消息。
    *   `UPDATE_ACHIEVEMENTS`:
        *   （待定）更新玩家的成就信息，并可能通过广播 `PLAYER_LIST` 来同步状态。
