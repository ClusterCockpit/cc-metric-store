JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" -d $'cpu_load,cluster=alex,hostname=a042,type=hwthread,type-id=0 value=35.0 1725827464642231296'

rm sample_fritz.txt
rm sample_alex.txt

while [ true ]; do
    echo "Alex Metrics for hwthread types and type-ids"
    timestamp="$(date '+%s')"
    echo "Timestamp : "+$timestamp
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904; do
            for id in {0..127}; do
                echo "$metric,cluster=alex,hostname=$hostname,type=hwthread,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_alex.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" --data-binary @sample_alex.txt

    echo "Fritz Metrics for hwthread types and type-ids"
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in f0201 f0202 f0203 f0204 f0205 f0206 f0207 f0208 f0209 f0210 f0211 f0212 f0213 f0214 f0215 f0217 f0218 f0219 f0220 f0221 f0222 f0223 f0224 f0225 f0226 f0227 f0228 f0229 f0230 f0231 f0232 f0233 f0234 f0235 f0236 f0237 f0238 f0239 f0240 f0241 f0242 f0243 f0244 f0245 f0246 f0247 f0248 f0249 f0250 f0251 f0252 f0253 f0254 f0255 f0256 f0257 f0258 f0259 f0260 f0261 f0262 f0263 f0264 f0378; do
            for id in {0..71}; do
                echo "$metric,cluster=fritz,hostname=$hostname,type=hwthread,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_fritz.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=fritz' -H "Authorization: Bearer $JWT" --data-binary @sample_fritz.txt

    rm sample_fritz.txt
    rm sample_alex.txt

    echo "Alex Metrics for accelerator types and type-ids"
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904; do
            for id in 00000000:49:00.0 00000000:0E:00.0 00000000:D1:00.0 00000000:90:00.0 00000000:13:00.0 00000000:96:00.0 00000000:CC:00.0 00000000:4F:00.0; do
                echo "$metric,cluster=alex,hostname=$hostname,type=accelerator,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_alex.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" --data-binary @sample_alex.txt

    rm sample_alex.txt

    echo "Alex Metrics for memoryDomain types and type-ids"
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904; do
            for id in {0..7}; do
                echo "$metric,cluster=alex,hostname=$hostname,type=memoryDomain,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_alex.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" --data-binary @sample_alex.txt

    rm sample_alex.txt

    echo "Alex Metrics for socket types and type-ids"
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904; do
            for id in {0..1}; do
                echo "$metric,cluster=alex,hostname=$hostname,type=socket,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_alex.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" --data-binary @sample_alex.txt

    echo "Fritz Metrics for socket types and type-ids"
    for metric in cpu_load cpu_user flops_any cpu_irq cpu_system ipc cpu_idle cpu_iowait core_power clock; do
        for hostname in f0201 f0202 f0203 f0204 f0205 f0206 f0207 f0208 f0209 f0210 f0211 f0212 f0213 f0214 f0215 f0217 f0218 f0219 f0220 f0221 f0222 f0223 f0224 f0225 f0226 f0227 f0228 f0229 f0230 f0231 f0232 f0233 f0234 f0235 f0236 f0237 f0238 f0239 f0240 f0241 f0242 f0243 f0244 f0245 f0246 f0247 f0248 f0249 f0250 f0251 f0252 f0253 f0254 f0255 f0256 f0257 f0258 f0259 f0260 f0261 f0262 f0263 f0264 f0378; do
            for id in {0..1}; do
                echo "$metric,cluster=fritz,hostname=$hostname,type=socket,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_fritz.txt
            done
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=fritz' -H "Authorization: Bearer $JWT" --data-binary @sample_fritz.txt

    rm sample_fritz.txt
    rm sample_alex.txt

    echo "Alex Metrics for nodes"
    for metric in cpu_irq cpu_load mem_cached net_bytes_in cpu_user cpu_idle nfs4_read mem_used nfs4_write nfs4_total ib_xmit ib_xmit_pkts net_bytes_out cpu_iowait ib_recv cpu_system ib_recv_pkts; do
        for hostname in a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904; do
            echo "$metric,cluster=alex,hostname=$hostname,type=node value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_alex.txt
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" --data-binary @sample_alex.txt

    echo "Fritz Metrics for nodes"
    for metric in cpu_irq cpu_load mem_cached net_bytes_in cpu_user cpu_idle nfs4_read mem_used nfs4_write nfs4_total ib_xmit ib_xmit_pkts net_bytes_out cpu_iowait ib_recv cpu_system ib_recv_pkts; do
        for hostname in f0201 f0202 f0203 f0204 f0205 f0206 f0207 f0208 f0209 f0210 f0211 f0212 f0213 f0214 f0215 f0217 f0218 f0219 f0220 f0221 f0222 f0223 f0224 f0225 f0226 f0227 f0228 f0229 f0230 f0231 f0232 f0233 f0234 f0235 f0236 f0237 f0238 f0239 f0240 f0241 f0242 f0243 f0244 f0245 f0246 f0247 f0248 f0249 f0250 f0251 f0252 f0253 f0254 f0255 f0256 f0257 f0258 f0259 f0260 f0261 f0262 f0263 f0264 f0378; do
            echo "$metric,cluster=fritz,hostname=$hostname,type=node value=$((1 + RANDOM % 100)).0 $timestamp" >>sample_fritz.txt
        done
    done

    curl -X 'POST' 'http://localhost:8082/api/write/?cluster=fritz' -H "Authorization: Bearer $JWT" --data-binary @sample_fritz.txt

    rm sample_fritz.txt
    rm sample_alex.txt

    sleep 1m
done
# curl -X 'POST' 'http://localhost:8081/api/write/?cluster=alex' -H "Authorization: Bearer $JWT" -d $'cpu_load,cluster=alex,hostname=a042,type=hwthread,type-id=0 value=35.0 1725827464642231296'