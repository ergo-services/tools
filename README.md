# Under development. 

# Tools that make your life easier working with Ergo Framework and Ergo Services.

- `ergo` - boilerplate code generator to create service with Ergo Framework. To install it use the following command:
   
   `go install ergo.services/tools/ergo@latest`
   
   Example of how to generate code of your node:
   
   `ergo -init myproject -with-app MyApp -with-sup MyApp:MySup -with-actor MySup:MyActor -with-msg MyMessage`
   
   as a result you will get generated project:
   ```
   myproject/            
   |-- apps/
   |   `-- myapp/
   |       |-- myactor.go
   |       |-- myapp.go
   |       `-- mysup.go
   |-- node.go
   `-- types.go
   ```
   
   See `ergo -help` for more information
