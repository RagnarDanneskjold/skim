pipeline {
  agent {
    node {
      label 'gce'
    }
    
  }
  stages {
    stage('Tests') {
      steps {
        sh '''go version
go test -cover "$PKG/..."'''
      }
    }
  }
  environment {
    PKG = 'go.spiff.io/skim'
  }
}