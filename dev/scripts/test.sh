eval $(go run create_and_fund.go)

# Now the environment variables are set in the shell
echo $STELLAR_PUBLIC_KEY
echo $STELLAR_SECRET_KEY
