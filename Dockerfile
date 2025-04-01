FROM golang:1.24

COPY . .

ENTRYPOINT ["./entrypoint.sh"]
