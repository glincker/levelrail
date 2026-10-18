const express = require('express')

const app = express()
app.get('/', (_req, res) => res.send('ok'))
app.listen(process.env.PORT || 3000, '0.0.0.0')
