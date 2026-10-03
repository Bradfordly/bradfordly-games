resource "aws_acm_certificate" "panel" {
  domain_name               = var.panel_hostname
  subject_alternative_names = ["*.${var.panel_hostname}"]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "panel_cert" {
  for_each = var.route53_zone_id == "" ? {} : {
    for dvo in aws_acm_certificate.panel.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = var.route53_zone_id
}

resource "aws_acm_certificate_validation" "panel" {
  count = var.route53_zone_id == "" ? 0 : 1

  certificate_arn         = aws_acm_certificate.panel.arn
  validation_record_fqdns = [for record in aws_route53_record.panel_cert : record.fqdn]
}
