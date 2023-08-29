#!/bin/bash

# 设置密码（请替换为实际密码）
password="qwer123"

# 运行go程序
go run .

# 等待5秒
sleep 5

# 切换到ansible目录的上级目录
cd ../ansible

# 使用密码执行ansible playbook以配置服务器
ansible-playbook -i ./hosts conf-server-config.yaml --extra-vars "ansible_become_pass=$password"

# 使用密码执行ansible playbook以启动服务器
ansible-playbook -i ./hosts run-server.yaml --extra-vars "ansible_become_pass=$password"

# 等待25秒
sleep 40

# 杀死trebiz进程
sudo killall trebiz

# 使用密码执行ansible playbook以获取结果
ansible-playbook -i ./hosts fetch-results.yaml --extra-vars "ansible_become_pass=$password"


cd ~/shareWithPC/code/go/trebiz/calc_results

python3 calc-latency-throughput.py

cat FinalResults.txt
