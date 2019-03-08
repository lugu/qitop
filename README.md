# qitop

List the most used methods.

## Installation

```
export CGO_ENABLED=0
go get github.com/lugu/qitop
```
## Usage

Local:

```
scp qitop nao@<robotip>:~
ssh nao@<robotip>
./qitop
```

Remote:

```
printf "nao\nnao\n" > ~/.qi-auth.conf
./qitop -qi-url tcps://10.0.164.195:9443
```
