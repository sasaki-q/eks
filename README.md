# eks-dd

EKS + Datadog + Argo CD を組み合わせたテスト用プロジェクト。Terraform で EKS クラスタ (Auto Mode)・Datadog Agent・Argo CD を構築し、Argo CD の GitOps 同期でテストアプリ (nginx) をデプロイする。

## 構成

```
infra/   Terraform 一式 (VPC, EKS クラスタ [Auto Mode], NAT Gateway, Datadog Agent, Argo CD)
         infra/datadog.yaml は Datadog Helm chart 用の values ファイル (templatefile でAPIキー等を埋め込む)
k8s/     テストアプリの Kubernetes manifest
         ingress.yaml : ロードバランサー定義 (IngressClassParams + IngressClass + ALB Ingress)
         nginx.yaml   : サービス定義 (nginx Deployment + ClusterIP Service)
           Argo CD がこのディレクトリを watch する
argocd/  Argo CD の Application 定義 (このリポジトリの k8s/ を同期対象にする)
Makefile よく使う操作をまとめたコマンド集
```

## アーキテクチャ

- EKS **Auto Mode**・private nodes 構成。ノードの起動・スケーリング・OS/CNI/CSIの管理はコントロールプレーンに委譲されており、Managed Node Group や AWS Load Balancer Controller / EBS CSI Driver の Helm chart を自前で管理する必要がない
  - `compute_config` (ノード自動プロビジョニング)・`kubernetes_network_config.elastic_load_balancing` (ALB/NLB)・`storage_config.block_storage` (EBS) の3つを有効化している
  - ノード用 IAM ロールは `AmazonEKSWorkerNodeMinimalPolicy` + `AmazonEC2ContainerRegistryPullOnly` のみで足りる (CNI/フルワーカー権限はコントロールプレーン側が保持)
  - クラスタ用 IAM ロールには `AmazonEKSClusterPolicy` に加え `AmazonEKSComputePolicy` / `AmazonEKSBlockStoragePolicy` / `AmazonEKSLoadBalancingPolicy` / `AmazonEKSNetworkingPolicy` が必要
- コントロールプレーンは public エンドポイントだが、`master_authorized_networks` で許可した送信元IPからのみアクセス可能 (`endpoint_private_access` も有効)
- `access_config.authentication_mode` は `API_AND_CONFIG_MAP`。旧来の CONFIG_MAP 専用クラスタは「作成者IAMプリンシパルが暗黙的に `system:masters` を持つ」という仕組みで動いていたが、これは CONFIG_MAP モードだけの特別扱いのため、モードを切り替えると同時に Terraform 実行者(呼び出し元IAMプリンシパル)へ `aws_eks_access_entry` + `aws_eks_access_policy_association` (`AmazonEKSClusterAdminPolicy`) で明示的にアクセス権を付与している。これが無いと切り替え直後に kubectl/helm 双方からロックアウトされる
  - `authentication_mode` は `CONFIG_MAP -> API_AND_CONFIG_MAP -> API` の順にしか遷移できず、逆方向には戻せない
- nginx の Pod は private subnet 上のノードに配置され、外部には直接晒されない。外部公開は **EKS Auto Mode が作成する ALB** のみが担う
  - ALB 自体 (ENI) は `kubernetes.io/role/elb` タグの付いた public subnet に配置される
  - Auto Mode はセルフマネージドの AWS Load Balancer Controller と異なり `IngressClass`/`IngressClassParams` を自動作成しないため、`k8s/ingress.yaml` で明示的に定義している (`controller: eks.amazonaws.com/alb`)。ALBのscheme (internet-facing/internal) は `IngressClassParams.spec.scheme` で指定し、`alb.ingress.kubernetes.io/scheme` アノテーションでは設定できない
  - `Ingress` に `alb.ingress.kubernetes.io/target-type: ip` を付けているため、ALB は Pod の IP に直接トラフィックを転送する (private subnet 上の Pod へ直配送。ノードのポート経由ではない)
  - IRSA/OIDC プロバイダは不要 (Auto Mode の ALB 機能はコントロールプレーンが直接処理するため)
- Datadog Agent (DaemonSet) と Cluster Agent を Helm 経由でデプロイ。値は `infra/datadog.yaml` に切り出し、`templatefile()` で APIキー・site・cluster名を埋め込んで渡している
- Argo CD を Helm 経由でデプロイし、GitHub リポジトリの `k8s/` を `syncPolicy.automated` (prune + selfHeal) で自動同期
  - 同期方式は Git polling (デフォルトで数分ごとにリポジトリを確認)。GitHub Actions 側の変更や Argo CD server の外部公開は不要
  - `Application` の CRD がインストールされるまで存在しないため、Terraform ではなく `kubectl apply` で `argocd/application.yaml` を適用する構成にしている
- Datadog Agent のような「クラスタアドオン」は Terraform (Helm provider) で管理し、nginx のような「アプリケーション」は Argo CD の GitOps で管理する、という役割分担にしている

## 前提

- `aws` CLI で認証済み (`aws configure` または SSO)
- Terraform >= 1.5
- `kubectl`
- Datadog アカウントと APIキー (Organization Settings > API Keys)

## セットアップ

1. 変数ファイルを作成する

   ```
   cp infra/terraform.tfvars.example infra/terraform.tfvars
   ```

   `infra/terraform.tfvars` を編集し、以下を設定する。

   | 変数 | 内容 |
   |---|---|
   | `master_authorized_networks` | kubectl/terraform を実行するマシンのグローバルIP (`"x.x.x.x/32"`) |
   | `datadog_api_key` | Datadog の APIキー (32桁英数字、ハイフンなし) |

   ※ `infra/terraform.tfvars` は `.gitignore` 済みで、コミットされない。
   ※ AWS の対象アカウント/リージョンは `aws` CLI の認証情報 (プロファイルや `AWS_PROFILE`) に従う。リージョンを変えたい場合は `infra/terraform.tfvars` に `region` を追加する。

2. EKS クラスタ (Auto Mode)・Datadog Agent・Argo CD を作成する

   ```
   make init
   make plan
   make apply
   ```

3. Argo CD の Application を登録する (`k8s/` を同期対象として登録)

   ```
   make apply-argocd
   ```

登録後は Argo CD が Git を自動ポーリングするため、`k8s/` を更新して `main` にマージすれば自動でクラスタに反映される。手動での `kubectl apply -f k8s/` は不要。

## テストアプリの動作確認

`kubectl get ingress nginx` で `ADDRESS` (ALB の DNS名) が割り当てられるのを待ち、ブラウザまたは `curl` でアクセス確認する。

```
make get-credentials
kubectl get application -n argocd
kubectl get ingress nginx
```

## Datadog での確認

```
kubectl get pods -n datadog
```

全ての Agent Pod が `Running` になったら、Datadog UI の Infrastructure > Kubernetes でクラスタ・ノード・Pod のメトリクスを、Logs で nginx のアクセスログを確認できる。

## private リポジトリにする場合

現在 `argocd/application.yaml` の `repoURL` は公開リポジトリを前提にしており、追加の認証設定は不要。リポジトリを private にする場合は、`argocd` namespace に `argocd.argoproj.io/secret-type: repository` ラベル付きの Secret を登録する必要がある(具体例は `argocd/application.yaml` 冒頭にコメントで記載)。`Application` 側は `repoURL` の一致で自動的にこの Secret と紐付くため、`application.yaml` 自体の変更は不要。

## Makefile コマンド一覧

| コマンド | 内容 |
|---|---|
| `make init` | terraform init |
| `make plan` | terraform plan |
| `make apply` | terraform apply (EKS Auto Mode + Datadog Agent + Argo CD を作成) |
| `make destroy` | terraform destroy (作成したリソースを削除) |
| `make fmt` | terraform fmt --recursive |
| `make list-state` | terraform state list |
| `make get-credentials` | kubectl の context を EKS クラスタに合わせる (`aws eks update-kubeconfig`) |
| `make apply-manifest` | context を合わせてから `kubectl apply -f k8s/` (手動デプロイ用。通常は Argo CD が自動同期する) |
| `make apply-argocd` | context を合わせてから `kubectl apply -f argocd/` (Application の登録・更新) |

`get-credentials` / `apply-manifest` / `apply-argocd` は Makefile 内の `CLUSTER_NAME` / `REGION` / `PROFILE` (デフォルトそれぞれ `eks-dd-test` / `ap-northeast-1` / `default`) を使うため、必要に応じて実行時に上書きすること (例: `make get-credentials PROFILE=myprofile`)。

## 注意事項

- `infra/terraform.tfvars` と `infra/terraform.tfstate*` には認証情報やAPIキーが含まれるため、`.gitignore` で除外している。実際の値は絶対にコミットしないこと
- 自分のグローバルIPが変わると `master_authorized_networks` の許可リストから外れ、kubectl/helm がコントロールプレーンにタイムアウトで接続できなくなる。その場合は `infra/terraform.tfvars` の値を更新して `make apply` を再実行する
- EKS Auto Mode のノードや NAT Gateway、ALB は課金対象のリソース。`make destroy` で確認なしにクラスタと関連リソースが削除される(テスト用途のための設定)
- `make destroy` の前に `kubectl delete ingress nginx` (または Argo CD の Application を削除) しておくこと。EKS Auto Mode が管理する ALB は Terraform 管理外のため、Ingress を残したままクラスタや VPC を先に破棄すると ALB が孤立(orphan)し、AWS コンソールから手動削除が必要になる
- `access_config.authentication_mode` を変更する際は、変更前に自分のIAMプリンシパル用の Access Entry が確実に存在することを確認すること。無い状態で `CONFIG_MAP` から抜けると、その場で kubectl/helm 両方からロックアウトされる(Access Entry の作成自体はIAM APIのみで完結するため、ロックアウト後も Terraform からの復旧は可能)
- Argo CD server は外部公開していない (ClusterIP のみ)。UI で確認したい場合は `kubectl -n argocd port-forward svc/argocd-server 8080:443` を使う
