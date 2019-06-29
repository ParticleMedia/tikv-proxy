#!/bin/bash -e
cd $(dirname $0)

clean_days=14

log_dir="../log"
if [ ! -d ${log_dir} ]; then
    echo "${log_dir} is not a dictionary"
    exit 1
fi
cd ${log_dir}

log_name="tikv_proxy"
cur_info_log=`readlink ${log_name}.INFO`
cur_warn_log=`readlink ${log_name}.WARNING`
cur_error_log=`readlink ${log_name}.ERROR`
cur_fatal_log=`readlink ${log_name}.FATAL`

max_diff=`expr ${clean_days} \* 86400`
cur_ts=`date +%s`

for file in `ls ${log_name}.*.log.*`; do
    if [ "x$file" == "x$cur_info_log" -o "x$file" == "x$cur_warn_log" -o "x$file" == "x$cur_error_log" -o "x$file" == "x$cur_fatal_log" ]; then
        continue
    fi

    file_date=`echo $file | sed -r 's/.*\.[A-Z]*\.([0-9]{8})-.*/\1/g'`
    file_ts=`date +%s -d "$file_date"`
    ts_diff=`expr ${cur_ts} - ${file_ts}`
    if [ ${ts_diff} -gt ${max_diff} ]; then
        echo "rm expire log ${file}"
        rm -f ${file} &>/dev/null
    fi
done
