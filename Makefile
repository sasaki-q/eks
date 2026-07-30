init:
	terraform -chdir=infra init -backend-config=backend.hcl -migrate-state

plan:
	terraform -chdir=infra plan

apply:
	terraform -chdir=infra apply --auto-approve

destroy:
	terraform -chdir=infra destroy

fmt:
	terraform fmt --recursive

list-state:
	terraform -chdir=infra state list

TARGET_STATE=dummy
RESOURCE_NAME=dummy
import-state:
	terraform -chdir=infra import ${TARGET_STATE} ${RESOURCE_NAME}

RM_TARGET_STATE=dummy
rm-state:
	terraform -chdir=infra state rm ${RM_TARGET_STATE}

CLUSTER_NAME=eks-dd-test
REGION=ap-northeast-1
PROFILE=dummy

sso-login:
	aws sso login --profile ${PROFILE}

get-credentials:
	aws eks update-kubeconfig --name ${CLUSTER_NAME} --region ${REGION} --profile ${PROFILE}

apply-manifest: get-credentials
	kubectl apply -f k8s/

apply-argocd: get-credentials
	kubectl apply -f argocd/

delete-manifest: get-credentials
	kubectl delete -f k8s/

delete-argocd: get-credentials
	kubectl delete -f argocd/

ECR_REPO=eks-dd
IMAGE_NAME=api
IMAGE_TAG=v1.0.0
ACCOUNT_ID=dummy
IMAGE=${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com/${ECR_REPO}/${IMAGE_NAME}:${IMAGE_TAG}

create-ecr-repo:
	aws ecr create-repository \
		--repository-name ${ECR_REPO}/${IMAGE_NAME} \
		--region ${REGION} \
		--profile ${PROFILE}

ecr-login:
	aws ecr get-login-password --region ${REGION} --profile ${PROFILE} | docker login --username AWS --password-stdin ${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com

build:
	docker buildx build -t ${IMAGE} -f cmd/Dockerfile cmd

push: build ecr-login
	docker push ${IMAGE}
