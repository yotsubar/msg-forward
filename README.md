# msg-forward

A websocket server forwarding messages sent by [msg-forward-ui](https://github.com/yotsubar/msg-forward-ui).

## Build

```shell
go build -o msgforward
```

## Run

Create a file `config.json` under the same directory of the binary.

```json
{
    "addr": ":8080",
    "username": "go",
    "password": "go"
}
```

Then

```shell
./msgforward
```

## Acknowledgments

[gws](https://github.com/lxzan/gws)