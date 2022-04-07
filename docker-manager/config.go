package docker_manager

var xdebugConf = `xdebug.remote_enable=1
xdebug.remote_autostart=1
xdebug.remote_connect_back=0
xdebug.remote_handler = dbgp
xdebug.remote_mode = req
xdebug.remote_host=%s
xdebug.remote_port=%d`
