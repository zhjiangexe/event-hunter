Feature: Insert one JSONEachRow document into ClickHouse

  Scenario:
    Given url clickhouseHttpUrl
    And header Authorization = authorization
    And header Content-Type = 'application/json'
    And param database = 'event_hunter'
    And param query = 'INSERT INTO ' + table + ' FORMAT JSONEachRow'
    And request karate.toString(row) + '\n'
    When method post

