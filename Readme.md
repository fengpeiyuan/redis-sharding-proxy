Redis Sharding Proxy
======================


Introduction
------------

Most code are from https://github.com/smira/redis-resharding-proxy, but has new feature.
If your redis client is jedis and wand to do capacity expansion by sharding redis using jedis, this project is suit for you. 


Installing
-------------------

If you have Go environment ready::

    go get github.com/fengpeiyuan/redis-sharding-proxy

Otherwise install Go and set up environment:

    $ mkdir $HOME/go
    $ export GOPATH=$HOME/go
    $ export PATH=$PATH:$GOPATH/bin

After that you can run ``redis-sharding-proxy``.

Using
-----

``redis-sharding-proxy`` accepts several options:

  -master-host="masterhost"     Master Redis host
  -master-port=6379             Master Redis port
  -proxy-host="proxyhost"       Proxy listening interface, waiting for slave to connect.
  -proxy-port=6380              Proxy port waiting for slave to connect.
  -slave-host="slavehost"       Slave redis host.
  -slave-port=6381              Slave redis port.

Connect string is given as below:

    host1:port1,host2:port2,...,hostn:portn

Example
-------

Assume master host 192.168.0.1 port is 10000, we can divided it into five redis instances using five sharding proxy below:

proxy1: 192.168.0.2:11001

proxy1: 192.168.0.2:11002

proxy1: 192.168.0.2:11003

proxy1: 192.168.0.2:11004

proxy1: 192.168.0.2:11005

We launch these five proxy using::

./redis-sharding-proxy -master-host=192.168.0.1 -master-port=10000 -proxy-host=192.168.0.2 -proxy-port=11001 -slave-host=192.168.0.3 -slave-port=10001 192.168.0.3:10001,192.168.0.3:10002,192.168.0.3:10003,192.168.0.3:10004,192.168.0.3:10005

./redis-sharding-proxy -master-host=192.168.0.1 -master-port=10000 -proxy-host=192.168.0.2 -proxy-port=11002 -slave-host=192.168.0.3 -slave-port=10002 192.168.0.3:10001,192.168.0.3:10002,192.168.0.3:10003,192.168.0.3:10004,192.168.0.3:10005

./redis-sharding-proxy -master-host=192.168.0.1 -master-port=10000 -proxy-host=192.168.0.2 -proxy-port=11003 -slave-host=192.168.0.3 -slave-port=10003 192.168.0.3:10001,192.168.0.3:10002,192.168.0.3:10003,192.168.0.3:10004,192.168.0.3:10005

./redis-sharding-proxy -master-host=192.168.0.1 -master-port=10000 -proxy-host=192.168.0.2 -proxy-port=11004 -slave-host=192.168.0.3 -slave-port=10004 192.168.0.3:10001,192.168.0.3:10002,192.168.0.3:10003,192.168.0.3:10004,192.168.0.3:10005

./redis-sharding-proxy -master-host=192.168.0.1 -master-port=10000 -proxy-host=192.168.0.2 -proxy-port=11005 -slave-host=192.168.0.3 -slave-port=10005 192.168.0.3:10001,192.168.0.3:10002,192.168.0.3:10003,192.168.0.3:10004,192.168.0.3:10005

Five slaves are::

slave1: 192.168.0.3:10001 then type slaveof 192.168.0.2 11001

slave1: 192.168.0.3:10002 then type slaveof 192.168.0.2 11002

slave1: 192.168.0.3:10003 then type slaveof 192.168.0.2 11003

slave1: 192.168.0.3:10004 then type slaveof 192.168.0.2 11004

slave1: 192.168.0.3:10005 then type slaveof 192.168.0.2 11005





Thanks
------

I would like to say thanks for Andrey Smirnov.


