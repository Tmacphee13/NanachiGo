printf "~~~~~~~~~~~~~~~ Authentication Tests ~~~~~~~~~~~~~~~\n"
go test -v ./internal/auth
echo ""

printf "~~~~~~~~~~~~~~~ Server Setup Tests ~~~~~~~~~~~~~~~\n"
go test -v ./internal/server
echo ""
