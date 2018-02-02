#!/bin/bash
# @author Michal Karm Babacek

# @See https://www.unbound.net/documentation/howto_optimise.html

export NUM_OF_MY_CPUS="`nproc`"
export UNBOUND_CONFIG_FILE="/etc/unbound/unbound.conf"
export UNBOUND_FORWARD_CONFIG_FILE="/etc/unbound/conf.d/forward.conf"

# Credit: John1024, SO
pw2() { echo "x=l($1)/l(2); scale=0; 2^((x+0.5)/1)" | bc -l; }

if [[ "${NUM_OF_MY_CPUS}" -lt 2 ]]; then
    echo "Too few CPUs. Use at least 2 cores, recommended 4 and more."
fi

if [[ "${NUM_OF_MY_CPUS}" -le 4 ]]; then
    echo "With just 4 CPUs, we consider this box to be a test machine; using all cores."
    export UNBOUND_NUM_THREADS=${NUM_OF_MY_CPUS}
    export SINKIT_NUM_OF_CPUS=${NUM_OF_MY_CPUS}
else
    # If the number is odd, the +1 goes to GoDNS
    export UNBOUND_NUM_THREADS=$((${NUM_OF_MY_CPUS}/2))
    export SINKIT_NUM_OF_CPUS=$((${NUM_OF_MY_CPUS}/2 + (${NUM_OF_MY_CPUS} % 2)))
fi

UB_SLABS=`pw2 ${UNBOUND_NUM_THREADS}`
export UNBOUND_INFRA_CACHE_SLABS=${UB_SLABS}
export UNBOUND_RRSET_CACHE_SLABS=${UB_SLABS}
export UNBOUND_MSG_CACHE_SLABS=${UB_SLABS}

TOTAL_RAM_K=$(free -k | awk '/^Mem:/{print $2}')
TOTAL_RAM_B=$(($TOTAL_RAM_K*1024))

# Unbound should use up to n % of total RAM
UNBOUND_MAX_PERCENT=${UNBOUND_MAX_PERCENT:-30}

# GoDNS should use up to n % of total RAM
GODNS_MAX_PERCENT=${GODNS_MAX_PERCENT:-30}

# GoDNS oraculum
HOW_MUCH_ONE_CACHE_RECORD_TAKES_B=1000
export SINKIT_ORACULUM_CACHE_MAXCOUNT=`echo "scale=0; ($TOTAL_RAM_B/100*$GODNS_MAX_PERCENT)/$HOW_MUCH_ONE_CACHE_RECORD_TAKES_B;" | bc`

UB_RAM_K=`echo "scale=0; $TOTAL_RAM_K/100*$UNBOUND_MAX_PERCENT;" | bc`

# See the Unbound doc, mind malloc overhead
export UNBOUND_RRSET_CACHE_SIZE="$(($UB_RAM_K/2))k"
export UNBOUND_MSG_CACHE_SIZE="$(($UB_RAM_K/4))k"

# Hard limits that trigger process restart
# If either of these eats up to:
HARD_MEM_LIMIT_PERCENT=${HARD_MEM_LIMIT_PERCENT:-60}
export GODNS_HARD_RAM_LIMIT="`echo "scale=0; $TOTAL_RAM_K/100*$HARD_MEM_LIMIT_PERCENT;" | bc`KB"
export UNBOUND_HARD_RAM_LIMIT=${GODNS_HARD_RAM_LIMIT}

sed -i "s~@UNBOUND_LOGLEVEL@~${UNBOUND_LOGLEVEL:-1}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_LOG_QUERIES@~${UNBOUND_LOG_QUERIES:-no}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_NUM_THREADS@~${UNBOUND_NUM_THREADS:-4}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_OUTGOING_RANGE@~${UNBOUND_OUTGOING_RANGE:-32768}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_NUM_QUERIES_PER_THREAD@~${UNBOUND_NUM_QUERIES_PER_THREAD:-4096}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_JOSTLE_TIMEOUT@~${UNBOUND_JOSTLE_TIMEOUT:-1000}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_OUTGOING_NUM_TCP@~${UNBOUND_OUTGOING_NUM_TCP:-10}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_INCOMING_NUM_TCP@~${UNBOUND_INCOMING_NUM_TCP:-10}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_SO_RCVBUF@~${UNBOUND_SO_RCVBUF:-32m}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_SO_SNDBUF@~${UNBOUND_SO_SNDBUF:-32m}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_CACHE_MIN_TTL@~${UNBOUND_CACHE_MIN_TTL:-60}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_CACHE_MAX_TTL@~${UNBOUND_CACHE_MAX_TTL:-86400}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_CACHE_MAX_NEGATIVE_TTL@~${UNBOUND_CACHE_MAX_NEGATIVE_TTL:-3600}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_INFRA_CACHE_SLABS@~${UNBOUND_INFRA_CACHE_SLABS:-16}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_INFRA_CACHE_NUMHOSTS@~${UNBOUND_INFRA_CACHE_NUMHOSTS:-120000}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_EDNS_BUFFER_SIZE@~${UNBOUND_EDNS_BUFFER_SIZE:-4096}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_MAX_UDP_SIZE@~${UNBOUND_MAX_UDP_SIZE:-4096}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_MSG_BUFFER_SIZE@~${UNBOUND_MSG_BUFFER_SIZE:-65552}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_RRSET_CACHE_SIZE@~${UNBOUND_RRSET_CACHE_SIZE:-512m}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_RRSET_CACHE_SLABS@~${UNBOUND_RRSET_CACHE_SLABS:-8}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_MSG_CACHE_SIZE@~${UNBOUND_MSG_CACHE_SIZE:-512m}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_MSG_CACHE_SLABS@~${UNBOUND_MSG_CACHE_SLABS:-8}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_DO_IP6@~${UNBOUND_DO_IP6:-yes}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_DO_IP4@~${UNBOUND_DO_IP4:-yes}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_DO_UDP@~${UNBOUND_DO_UDP:-yes}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_DO_TCP@~${UNBOUND_DO_TCP:-yes}~g" ${UNBOUND_CONFIG_FILE}
sed -i "s~@UNBOUND_MODULE_CONFIG@~${UNBOUND_MODULE_CONFIG:-iterator}~g" ${UNBOUND_CONFIG_FILE}

#setup forwarding from unbound
touch $UNBOUND_FORWARD_CONFIG_FILE && rm $UNBOUND_FORWARD_CONFIG_FILE
if [[ -z "${UNBOUND_BACKEND_RESOLVERS}" ]]; then
   echo "UNBOUND_BACKEND_RESOLVERS is unset, unbound will do the recursive resolution"
   else
   echo "UNBOUND_BACKEND_RESOLVERS is set to $UNBOUND_BACKEND_RESOLVERS, unbound will forward queries"
   echo "forward-zone:" > $UNBOUND_FORWARD_CONFIG_FILE
   echo "    name: \".\"" >> $UNBOUND_FORWARD_CONFIG_FILE
   for i in $(echo $UNBOUND_BACKEND_RESOLVERS | tr "," "\n")
   do
     addr=`echo $i | tr ":" "@"`
     echo "    forward-addr: $addr" >> $UNBOUND_FORWARD_CONFIG_FILE
   done
fi

#unbound trust anchor intialization
unbound-anchor

# Start
exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf -n
