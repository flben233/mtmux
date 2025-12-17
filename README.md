# Multiple Tunnels Multiplexing (mtmux)

mtmux is a Go library that enables multiplexing multiple logical streams over multiple TCP tunnels. It allows for efficient data transfer by distributing the load across several connections, improving performance and reliability.

```mermaid
graph LR
    S1[Stream 1]
    S2[Stream 2]
    S3[Stream 3]
    S4[Stream 4]
    S5[Stream 5]
    S6[Stream 6]

    Sp1[Stream 1']
    Sp2[Stream 2']
    Sp3[Stream 3']
    Sp4[Stream 4']
    Sp5[Stream 5']
    Sp6[Stream 6']

    A((A))
    B((B))

    subgraph Middle[Multiplexer]
        L1[Link 1]
        L2[Link 2]
        L3[Link 3]
    end

    S1 --> A
    S2 --> A
    S3 --> A
    S4 --> A
    S5 --> A
    S6 --> A

    A --> L1
    A --> L2
    A --> L3

    L1 --> B
    L2 --> B
    L3 --> B

    B --> Sp1
    B --> Sp2
    B --> Sp3
    B --> Sp4
    B --> Sp5
    B --> Sp6

    %% 定义样式：所有文本为黑色
    classDef stream fill:#e6f3ff,stroke:#3399ff,color:black;
    classDef link fill:#fff2e6,stroke:#ff9933,color:black;
    classDef point fill:#f0f0f0,stroke:#666,stroke-width:2px,color:black;
    classDef subgraphLabel color:black;

    %% 应用样式
    class S1,S2,S3,S4,S5,S6,Sp1,Sp2,Sp3,Sp4,Sp5,Sp6 stream
    class L1,L2,L3 link
    class A,B point

    %% 强制子图标题也为黑色（部分渲染器支持）
    class Left,Right,Middle subgraphLabel
```

## Example

Server:

```go
// Get a default configuration
cfg := mtmux.DefaultConfig()

// Listen for incoming connections. This will use the port range from 12345 to 12345 + Tunnels - 1
ln, _ := mtmux.Listen("tcp", "127.0.0.1:12345", int(cfg.Tunnels))
defer ln.Close()
bundle, _ := ln.Accept()

// Create a new server session
session, _ := mtmux.Server(bundle, cfg)
session.Start(context.Background())
defer session.Close()

// Accept a new stream
stream, _ := session.AcceptStream()

// Write and read some data
stream.Write([]byte("hello"))
buf := make([]byte, 1024)
n, _ := stream.Read(buf)

fmt.Println(string(buf[:n]))
```

Client:

```go
// Get a default configuration
cfg := mtmux.DefaultConfig()

// Dial to the server
bundle, _ := mtmux.Dial("tcp", "127.0.0.1:12345", int(cfg.Tunnels))

// Create a new client session
session, _ := mtmux.Client(bundle, cfg)
session.Start(context.Background())
defer session.Close()

// Open a new stream
stream, _ := session.OpenStream()

// Write and read some data
buf := make([]byte, 1024)
n, _ := stream.Read(buf)
_, _ = stream.Write([]byte("world"))

fmt.Println(string(buf[:n]))
```

## Configuration

- `Tunnels`: Number of TCP tunnels to use for multiplexing.
- `KeepAliveInterval`: Interval for sending keep-alive messages.
- `KeepAliveTimeout`: Timeout duration for keep-alive messages.
- `Timeout`: Timeout duration for waiting for missing data frames.
