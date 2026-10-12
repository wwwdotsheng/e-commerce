#!/usr/bin/env bash
# 应用 Secret。优先用 secret.local.yaml（含真值），没有就提示。
#
#   ./scripts/apply-secrets.sh              # 应用全部
#   ./scripts/apply-secrets.sh app          # 只应用 app 一个
#
# 为什么不用 kubectl apply -k：
#   kustomization.yaml 刻意不引用 secret.yaml，
#   所以 apply -k 不会碰 Secret —— 集群里的真值不会被模板里的占位符覆盖。
#   代价是 Secret 要单独应用，就是这个脚本干的事。
set -uo pipefail

MODULES=(app postgres redis rabbitmq)
[ $# -gt 0 ] && MODULES=("$@")

rc=0
for m in "${MODULES[@]}"; do
  tpl="k8s/$m/secret.yaml"
  loc="k8s/$m/secret.local.yaml"

  if [ ! -f "$tpl" ]; then
    echo "  ⚠️  $m: 找不到 $tpl"; rc=1; continue
  fi

  if [ -f "$loc" ]; then
    f="$loc"
  else
    echo "  ❌ $m: 没有 $loc"
    echo "         cp $tpl $loc  然后填入真实值"
    echo "         （$loc 已在 .gitignore，不会被提交）"
    rc=1
    continue
  fi

  # 检查里面还有没有占位符
  if grep -qE 'replace-with|CHANGE_ME' "$f" 2>/dev/null; then
    echo "  ⚠️  $m: $f 里还有占位符，确认要应用吗？（3 秒后继续，Ctrl-C 取消）"
    sleep 3
  fi

  if out=$(kubectl apply -f "$f" 2>&1); then
    echo "  ✅ $m: $out"
  else
    echo "  ❌ $m: $out"; rc=1
  fi
done

echo
if [ $rc -eq 0 ]; then
  echo "全部应用完成。"
  echo
  echo "⚠️  记得：改 Secret 要改 secret.local.yaml，不要直接 kubectl edit"
  echo "    否则下次跑这个脚本会被 .local.yaml 的值覆盖回去。"
else
  echo "有失败项，见上面。"
fi
exit $rc
