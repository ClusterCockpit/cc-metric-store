JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# If the collector and store and nats-server have been running for at least 60 seconds on the same host, you may run:
curl -X 'POST' 'http://localhost:8082/api/free/?to=1724536800' -H "Authorization: Bearer $JWT" -d "[     [    \"alex\",     \"a0329\",         \"memoryDomain2\"     ],     [        \"alex\", \"a0903\"     ],[        \"fritz\", \"f0201\"     ]  ]"

# ...
