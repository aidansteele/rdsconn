# This app no longer works. The AWS service now blocks any ports other than 22 (SSH) and 3389 (RDP). See issue [#5](https://github.com/aidansteele/rdsconn/issues/5)

# rdsconn

On June 14th, 2023 AWS [launched][aws-blog] new connectivity options for
_EC2 Instance Connect_. This functionality also works for non-EC2 resources in
VPCs. You _could_ run the official AWS CLI (>= v2.12.0) using the following command,
but `rdsconn` aims to make the RDS experience easier.

```
aws ec2-instance-connect open-tunnel \
  --private-ip-address 10.1.2.150 \
  --instance-connect-endpoint-id eice-06d8b7ad48example \
  --remote-port 5432 \
  --local-port 5432
```

## Installation

On macOS, `brew install aidansteele/taps/rdsconn`. On other platforms: see 
published binaries in the releases tab of the GitHub repo.

## Usage

1. Create an EC2 Instance Connect endpoint in your VPC. Ensure that your RDS DB 
   instance's security group allows the EIC endpoint to connect to it. 
2. Have valid AWS credentials configured. E.g. either as environment variables,
   default credentials in your config file, or a profile with `AWS_PROFILE=name` 
   env var set.
3. Run `rdsconn proxy`. The CLI will prompt you to select an RDS DB instance from
   the list of DBs in your account. Hit enter to confirm selection.
4. The message `Proxy running. Now waiting to serve connections to localhost:5432...` 
   will appear. You can now run `psql ... -h 127.0.0.1` (or `mysql ...`)

## Future plans

* Flesh out this README more
* Detect incorrect configurations and provide helpful error messages to user. 
  E.g. missing endpoints, security groups, routes, etc.
* Add a `client` subcommand that uses RDS IAM authentication to launch and
  authenticate a child process `psql` CLI (using PGPASSWORD etc env vars)

[aws-blog]: https://aws.amazon.com/blogs/compute/secure-connectivity-from-public-to-private-introducing-ec2-instance-connect-endpoint-june-13-2023/
