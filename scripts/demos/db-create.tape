Source scripts/demos/lib.tape
Output docs/assets/demos/db-create.gif
Sleep 600ms
Type "levelrail-cli databases create --name orders-db --engine postgres --version 16" Enter
Sleep 3s
Type "levelrail-cli databases get orders-db" Enter
Sleep 3s
Type `levelrail-cli databases query orders-db --sql "select * from orders limit 3"` Enter
Sleep 4s
