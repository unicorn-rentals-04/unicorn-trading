variable "vpc_id" {
  type = string
}

resource "random_string" "random" {
  length  = 10
  special = false
  upper   = false
}

locals {
  name_suffix = random_string.random.result
}

resource "aws_security_group" "reporter-all-intra-traffic" {
  name   = "ecomm_reporter_internal-traffic"
  vpc_id = var.vpc_id

  ingress {
    protocol  = "ALL"
    self      = true
    from_port = 0
    to_port   = 0
  }
}

# EC2 IAM role setup
resource "aws_iam_policy" "unicorn_rentals_policy" { // ec2 policy
  name = "unicorn_rentals_policy"
  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : "*",
        "Resource" : "*"
      }
    ]
  })
}

resource "aws_iam_role" "unicorn_rentals_role" { // assume role
  name = "unicorn_rentals_role"
  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Principal" : {
          "Service" : "ec2.amazonaws.com"
        },
        "Action" : "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_policy_attachment" "unicorn_rentals_role_attachment" {
  name       = "unicorn_rentals_role_attach"
  roles      = [aws_iam_role.unicorn_rentals_role.id]
  policy_arn = aws_iam_policy.unicorn_rentals_policy.id
}

resource "aws_iam_instance_profile" "unicorn_rentals_instance_profile" {
  name = "unicorn_rentals_instance_profile"
  role = aws_iam_role.unicorn_rentals_role.id
}

output "security_group" {
  value = aws_security_group.reporter-all-intra-traffic.id
}

output "instance_profile" {
  value = aws_iam_instance_profile.unicorn_rentals_instance_profile.id
}

output "name_suffix" {
  value = local.name_suffix
}
