package main
import ("fmt";"log";"net/http";"os";"github.com/stockyard-dev/stockyard-telegraph/internal/server";"github.com/stockyard-dev/stockyard-telegraph/internal/store")
func main(){port:=os.Getenv("PORT");if port==""{port="9700"};dataDir:=os.Getenv("DATA_DIR");if dataDir==""{dataDir="./telegraph-data"}
db,err:=store.Open(dataDir);if err!=nil{log.Fatalf("telegraph: %v",err)};defer db.Close();srv:=server.New(db,server.DefaultLimits())
fmt.Printf("\n  Telegraph — Self-hosted notification hub\n  Dashboard:  http://localhost:%s/ui\n  API:        http://localhost:%s/api\n  Questions? hello@stockyard.dev — I read every message\n\n",port,port)
log.Printf("telegraph: listening on :%s",port);log.Fatal(http.ListenAndServe(":"+port,srv))}
