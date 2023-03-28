# Setup an AWS instance
1. setup a local kube cluster in docker
2. mount crossplane onto that local kube cluster (cross plane will be responsible for interecting with s3 bucket on aws)
3. mount localplane onto that local kube cluster (localplane will emulate aws s3)
4. enable crossplane to provision aws s3
5. enable an instance of aws s3 bucket resource (many buckets can be in aws s3)