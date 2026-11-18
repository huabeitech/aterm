package main

// Collection script is shipped with this plugin, not the application.
const resourceProbeCommand = `LC_ALL=C; export LC_ALL
if [ "$(uname -s)" != Linux ]; then printf 'ATERM_UNSUPPORTED\n'; exit 0; fi
printf 'ATERM_SYSTEM\n'
hostname 2>/dev/null
uname -r 2>/dev/null
sed -n '/^PRETTY_NAME=/p' /etc/os-release 2>/dev/null
printf 'ATERM_UPTIME\n'
cat /proc/uptime 2>/dev/null
printf 'ATERM_CPU\n'
head -n 65 /proc/stat 2>/dev/null
printf 'ATERM_CPUCOUNT\n'
getconf _NPROCESSORS_ONLN 2>/dev/null || grep -c '^cpu[0-9]' /proc/stat 2>/dev/null
printf 'ATERM_LOAD\n'
cat /proc/loadavg 2>/dev/null
printf 'ATERM_MEMORY\n'
cat /proc/meminfo 2>/dev/null
printf 'ATERM_SWAPIO\n'
awk '/^pswpin / || /^pswpout / { print }' /proc/vmstat 2>/dev/null
printf 'ATERM_PAGESIZE\n'
getconf PAGESIZE 2>/dev/null
printf 'ATERM_DISK\n'
(df -PTkl 2>/dev/null || df -Pkl 2>/dev/null || df -PTk 2>/dev/null || df -Pk 2>/dev/null) | head -n 65
printf 'ATERM_INODES\n'
(df -PTil 2>/dev/null || df -Pil 2>/dev/null || df -PTi 2>/dev/null || df -Pi 2>/dev/null) | head -n 65
printf 'ATERM_NETWORK\n'
head -n 34 /proc/net/dev 2>/dev/null
printf 'ATERM_ROUTE\n'
ip -o route show default 2>/dev/null | head -n 1
printf 'ATERM_DISKIO\n'
head -n 65 /proc/diskstats 2>/dev/null
printf 'ATERM_END\n'
`
