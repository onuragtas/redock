package docker_manager

var xdebugConf = `zend_extension=xdebug.so
xdebug.remote_enable=1
xdebug.remote_autostart=1
xdebug.remote_connect_back=
xdebug.remote_handler = dbgp
xdebug.remote_mode = req
xdebug.remote_host=%s
xdebug.remote_port=%d`

var xdebugConf8 = `
zend_extension=xdebug.so
xdebug.mode=debug
xdebug.client_host=%s
xdebug.start_with_request=yes
xdebug.client_port=%d
xdebug.discover_client_host=true
`
