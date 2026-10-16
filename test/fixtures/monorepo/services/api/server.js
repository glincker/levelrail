const http = require('node:http')
const port = Number(process.env.PORT || 3000)

http
  .createServer((_req, res) => {
    res.end('api ok\n')
  })
  .listen(port)
