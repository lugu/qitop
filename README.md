# QiTop

List the most used methods and display agregated metrics.

![Screenshot](qitop.png)


## Navigation

    j/k or up/down : naviate the top list
    enter: visuallize the selected method
    space/backspace : scroll the logs
    page up/page down : navigate the logs

## Installation

    env CGO_ENABLED=0 go get github.com/lugu/qitop

## Usage

    $ qitop -h
    Usage of qitop:
      -log-file string
            file where to write qitop logs
      -log-level int
            log level, 1:fatal, 2:error, 3:warning, 4:info, 5:verbose, 6:debug (default 5)
      -method string
            method name
      -qi-url string
            Service directory URL (default "tcp://localhost:9559")
      -service string
            service name
      -token string
            user token
      -user string
            user name

## Credentials

One can create a file ~/.qiloop-auth.conf with the user and token.

## Credits

Based on [Termdash](http://github.com/mum4k/termdash/wiki) and
[QiLoop](http://github.com/lugu/qiloop).
