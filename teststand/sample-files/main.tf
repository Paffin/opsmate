provider "aws" {
  region = "us-east-1"
}

resource "aws_instance" "web" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"

  tags = {
    Name = "test-instance"
  }
}

resource "aws_s3_bucket" "data" {
  bucket = "my-test-bucket"

  tags = {
    Environment = "test"
  }
}
