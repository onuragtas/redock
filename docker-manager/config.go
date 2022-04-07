package docker_manager

var xdebugConf = `zend_extension=xdebug.so
xdebug.remote_enable=1
xdebug.remote_autostart=1
xdebug.remote_connect_back=
xdebug.remote_handler = dbgp
xdebug.remote_mode = req
xdebug.remote_host=%s
xdebug.remote_port=%d`
