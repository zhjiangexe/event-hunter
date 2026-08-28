Feature: Grafana deep-link targets match provisioned observability assets

  Background:
    * def basicAuthorization =
      """
      function(user, password) {
        var Base64 = Java.type('java.util.Base64');
        var JavaString = Java.type('java.lang.String');
        var StandardCharsets = Java.type('java.nio.charset.StandardCharsets');
        var value = new JavaString(user + ':' + password).getBytes(StandardCharsets.UTF_8);
        return 'Basic ' + Base64.getEncoder().encodeToString(value);
      }
      """
    * def authorization = basicAuthorization(grafanaUser, grafanaPassword)

  Scenario Outline: Deep-link datasource UID exists in Grafana
    Given url grafanaBaseUrl + '/api/datasources/uid/<uid>'
    And header Authorization = authorization
    When method get
    Then status 200
    And match response.uid == '<uid>'

    Examples:
      | uid        |
      | clickhouse |
      | tempo      |
      | loki       |

  Scenario: Quality Dashboard deep-link UID exists in Grafana
    Given url grafanaBaseUrl + '/api/dashboards/uid/event-quality'
    And header Authorization = authorization
    When method get
    Then status 200
    And match response.dashboard.uid == 'event-quality'

  Scenario: Alerting deep-link UID exists in Grafana
    Given url grafanaBaseUrl + '/api/v1/provisioning/alert-rules/event-quality-delay'
    And header Authorization = authorization
    When method get
    Then status 200
    And match response.uid == 'event-quality-delay'
